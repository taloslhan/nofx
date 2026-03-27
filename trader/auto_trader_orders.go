package trader

import (
	"fmt"
	"nofx/kernel"
	"nofx/logger"
	"nofx/market"
	"nofx/store"
	"strconv"
	"time"
)

// executeDecisionWithRecord executes AI decision and records detailed information
func (at *AutoTrader) executeDecisionWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	switch decision.Action {
	case "open_long":
		return at.executeOpenLongWithRecord(decision, actionRecord)
	case "open_short":
		return at.executeOpenShortWithRecord(decision, actionRecord)
	case "close_long":
		return at.executeCloseLongWithRecord(decision, actionRecord)
	case "close_short":
		return at.executeCloseShortWithRecord(decision, actionRecord)
	case "hold", "wait":
		// No execution needed, just record
		return nil
	default:
		return fmt.Errorf("unknown action: %s", decision.Action)
	}
}

type closePositionSnapshot struct {
	normalizedSymbol string
	entryPrice       float64
	quantity         float64
	entryTimeMs      int64
	currentPnLPct    float64
}

func extractUnixMillis(value interface{}) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case string:
		ms, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return ms
		}
	}
	return 0
}

func (at *AutoTrader) loadClosePositionSnapshot(symbol string, side string) *closePositionSnapshot {
	normalizedSymbol := market.Normalize(symbol)
	snapshot := &closePositionSnapshot{
		normalizedSymbol: normalizedSymbol,
	}

	dbSide := "LONG"
	if side == "short" {
		dbSide = "SHORT"
	}

	if at.store != nil {
		if openPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, normalizedSymbol, dbSide); err == nil && openPos != nil {
			snapshot.quantity = openPos.Quantity
			snapshot.entryPrice = openPos.EntryPrice
			snapshot.entryTimeMs = openPos.EntryTime
			logger.Infof("  📊 Using local position data: qty=%.8f, entry=%.2f", snapshot.quantity, snapshot.entryPrice)
		}
	}

	positions, err := at.trader.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if pos["symbol"] != symbol || pos["side"] != side {
				continue
			}

			if snapshot.entryPrice == 0 {
				if ep, ok := pos["entryPrice"].(float64); ok {
					snapshot.entryPrice = ep
				}
			}

			if snapshot.quantity == 0 {
				if amt, ok := pos["positionAmt"].(float64); ok {
					snapshot.quantity = amt
					if snapshot.quantity < 0 {
						snapshot.quantity = -snapshot.quantity
					}
				}
			}

			if snapshot.entryTimeMs == 0 {
				snapshot.entryTimeMs = extractUnixMillis(pos["createdTime"])
			}

			unrealizedPnl, _ := pos["unRealizedProfit"].(float64)
			markPrice, _ := pos["markPrice"].(float64)
			leverage := 10.0
			if lev, ok := pos["leverage"].(float64); ok && lev > 0 {
				leverage = lev
			}
			if snapshot.quantity > 0 && markPrice > 0 && leverage > 0 {
				marginUsed := (snapshot.quantity * markPrice) / leverage
				snapshot.currentPnLPct = calculatePnLPercentage(unrealizedPnl, marginUsed)
			}
			break
		}
	} else {
		logger.Infof("  ⚠️ Failed to get exchange position snapshot: %v", err)
	}

	if snapshot.entryTimeMs == 0 {
		posKey := symbol + "_" + side
		if firstSeen, exists := at.positionFirstSeenTime[posKey]; exists {
			snapshot.entryTimeMs = firstSeen
		}
	}

	return snapshot
}

func (at *AutoTrader) enforceCooldown(normalizedSymbol string) error {
	if at.config.StrategyConfig == nil {
		return nil
	}

	cooldownMinutes := at.config.StrategyConfig.RiskControl.CooldownMinutes
	if cooldownMinutes <= 0 {
		return nil
	}

	lastCloseTime := at.GetLastCloseTime(normalizedSymbol)
	if lastCloseTime.IsZero() {
		return nil
	}

	elapsed := time.Since(lastCloseTime)
	if elapsed < time.Duration(cooldownMinutes)*time.Minute {
		return fmt.Errorf("❌ [RISK CONTROL] Cooldown period active for %s (%d/%d min since last close)",
			normalizedSymbol, int(elapsed.Minutes()), cooldownMinutes)
	}

	return nil
}

func (at *AutoTrader) enforceMinHoldBeforeClose(symbol string, snapshot *closePositionSnapshot) error {
	if at.config.StrategyConfig == nil || snapshot == nil {
		return nil
	}

	minHoldMinutes := at.config.StrategyConfig.RiskControl.MinHoldMinutes
	if minHoldMinutes <= 0 || snapshot.entryTimeMs <= 0 {
		return nil
	}

	holdDuration := time.Since(time.UnixMilli(snapshot.entryTimeMs))
	if !shouldBlockActiveClose(
		holdDuration,
		minHoldMinutes,
		snapshot.currentPnLPct,
		at.config.StrategyConfig.RiskControl.EmergencyCloseLossPct,
	) {
		return nil
	}

	emergencyLossPct := normalizeEmergencyCloseLossPct(at.config.StrategyConfig.RiskControl.EmergencyCloseLossPct)
	return fmt.Errorf("❌ [RISK CONTROL] Min hold time not met for %s (%d/%d min), position PnL %.2f%% above emergency threshold %.2f%%",
		symbol, int(holdDuration.Minutes()), minHoldMinutes, snapshot.currentPnLPct, emergencyLossPct)
}

func applyReverseSLTPRecalculation(decision *kernel.Decision, marketPrice float64) bool {
	if decision == nil || marketPrice <= 0 {
		return false
	}
	if decision.OriginalStopLoss == 0 && decision.OriginalTakeProfit == 0 {
		return false
	}

	oldStopLoss := decision.StopLoss
	oldTakeProfit := decision.TakeProfit

	if decision.OriginalStopLoss > 0 {
		decision.StopLoss = 2*marketPrice - decision.OriginalStopLoss
	}
	if decision.OriginalTakeProfit > 0 {
		decision.TakeProfit = 2*marketPrice - decision.OriginalTakeProfit
	}

	logger.Infof("  🔄 [REVERSE] Recalculated SL/TP around market price %.4f (current stop: %.4f -> %.4f, current take: %.4f -> %.4f, original stop: %.4f, original take: %.4f)",
		marketPrice,
		oldStopLoss, decision.StopLoss,
		oldTakeProfit, decision.TakeProfit,
		decision.OriginalStopLoss, decision.OriginalTakeProfit,
	)

	return true
}

// executeOpenLongWithRecord executes open long position and records detailed information
func (at *AutoTrader) executeOpenLongWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  📈 Open long: %s", decision.Symbol)

	normalizedSymbol := market.Normalize(decision.Symbol)
	if err := at.enforceCooldown(normalizedSymbol); err != nil {
		return err
	}

	// ⚠️ Get current positions for multiple checks
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("failed to get positions: %w", err)
	}

	// [CODE ENFORCED] Check max positions limit
	if err := at.enforceMaxPositions(len(positions)); err != nil {
		return err
	}

	// Check if there's already a position in the same symbol and direction
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol && pos["side"] == "long" {
			return fmt.Errorf("❌ %s already has long position, close it first", decision.Symbol)
		}
	}

	// Get current price
	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}

	applyReverseSLTPRecalculation(decision, marketData.CurrentPrice)

	// Get balance (needed for multiple checks)
	balance, err := at.trader.GetBalance()
	if err != nil {
		return fmt.Errorf("failed to get account balance: %w", err)
	}
	availableBalance := 0.0
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Get equity for position value ratio check
	equity := 0.0
	if eq, ok := balance["totalEquity"].(float64); ok && eq > 0 {
		equity = eq
	} else if eq, ok := balance["totalWalletBalance"].(float64); ok && eq > 0 {
		equity = eq
	} else {
		equity = availableBalance // Fallback to available balance
	}

	// [CODE ENFORCED] Position Value Ratio Check: position_value <= equity × ratio
	adjustedPositionSize, wasCapped := at.enforcePositionValueRatio(decision.PositionSizeUSD, equity, decision.Symbol)
	if wasCapped {
		decision.PositionSizeUSD = adjustedPositionSize
	}

	// ⚠️ Auto-adjust position size if insufficient margin
	// Formula: totalRequired = positionSize/leverage + positionSize*0.001 + positionSize/leverage*0.01
	//        = positionSize * (1.01/leverage + 0.001)
	marginFactor := 1.01/float64(decision.Leverage) + 0.001
	maxAffordablePositionSize := availableBalance / marginFactor

	actualPositionSize := decision.PositionSizeUSD
	if actualPositionSize > maxAffordablePositionSize {
		// Use 98% of max to leave buffer for price fluctuation
		adjustedSize := maxAffordablePositionSize * 0.98
		logger.Infof("  ⚠️ Position size %.2f exceeds max affordable %.2f, auto-reducing to %.2f",
			actualPositionSize, maxAffordablePositionSize, adjustedSize)
		actualPositionSize = adjustedSize
		decision.PositionSizeUSD = actualPositionSize
	}

	// [CODE ENFORCED] Minimum position size check
	if err := at.enforceMinPositionSize(decision.PositionSizeUSD); err != nil {
		return err
	}

	// Calculate quantity with adjusted position size
	quantity := actualPositionSize / marketData.CurrentPrice
	actionRecord.Quantity = quantity
	actionRecord.Price = marketData.CurrentPrice

	// [CODE ENFORCED] Re-validate R:R with fresh market price before execution
	// This prevents stale-price validation from allowing bad trades
	// (e.g., cycle #190: AI analyzed at 86.81, but market moved to 88.5+ during AI processing)
	// Use 80% tolerance: market may drift during AI decision cycle, so allow slightly relaxed ratio
	if at.strategyEngine != nil {
		riskConfig := at.strategyEngine.GetRiskControlConfig()
		minRiskRewardRatio := riskConfig.MinRiskRewardRatio
		if hasDecisionTransform(decision, "reverse") {
			minRiskRewardRatio = at.getReverseMinRiskRewardRatio()
		}
		if err := kernel.ValidateRiskReward(decision.Action, marketData.CurrentPrice, decision.StopLoss, decision.TakeProfit, minRiskRewardRatio*0.8); err != nil {
			return fmt.Errorf("[pre-execution R:R check] %w", err)
		}
	}

	// Set margin mode
	if err := at.trader.SetMarginMode(decision.Symbol, at.config.IsCrossMargin); err != nil {
		logger.Infof("  ⚠️ Failed to set margin mode: %v", err)
		// Continue execution, doesn't affect trading
	}

	// Open position
	order, err := at.trader.OpenLong(decision.Symbol, quantity, decision.Leverage)
	if err != nil {
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}
	// Update entry price to actual fill price if available from exchange response
	if avgPrice, ok := order["avgPrice"].(float64); ok && avgPrice > 0 {
		actionRecord.Price = avgPrice
	}

	logger.Infof("  ✓ Position opened successfully, order ID: %v, quantity: %.4f", order["orderId"], quantity)

	if hasDecisionTransform(decision, "reverse") {
		at.markReversePosition(decision.Symbol, "long")
	}

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "open_long", quantity, marketData.CurrentPrice, decision.Leverage, 0)

	// Record position opening time
	posKey := decision.Symbol + "_long"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// Set stop loss and take profit
	if err := at.trader.SetStopLoss(decision.Symbol, "LONG", quantity, decision.StopLoss); err != nil {
		logger.Infof("  ⚠ Failed to set stop loss: %v", err)
	}
	if err := at.trader.SetTakeProfit(decision.Symbol, "LONG", quantity, decision.TakeProfit); err != nil {
		logger.Infof("  ⚠ Failed to set take profit: %v", err)
	}

	return nil
}

// executeOpenShortWithRecord executes open short position and records detailed information
func (at *AutoTrader) executeOpenShortWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  📉 Open short: %s", decision.Symbol)

	normalizedSymbol := market.Normalize(decision.Symbol)
	if err := at.enforceCooldown(normalizedSymbol); err != nil {
		return err
	}

	// ⚠️ Get current positions for multiple checks
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("failed to get positions: %w", err)
	}

	// [CODE ENFORCED] Check max positions limit
	if err := at.enforceMaxPositions(len(positions)); err != nil {
		return err
	}

	// Check if there's already a position in the same symbol and direction
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol && pos["side"] == "short" {
			return fmt.Errorf("❌ %s already has short position, close it first", decision.Symbol)
		}
	}

	// Get current price
	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}

	applyReverseSLTPRecalculation(decision, marketData.CurrentPrice)

	// Get balance (needed for multiple checks)
	balance, err := at.trader.GetBalance()
	if err != nil {
		return fmt.Errorf("failed to get account balance: %w", err)
	}
	availableBalance := 0.0
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Get equity for position value ratio check
	equity := 0.0
	if eq, ok := balance["totalEquity"].(float64); ok && eq > 0 {
		equity = eq
	} else if eq, ok := balance["totalWalletBalance"].(float64); ok && eq > 0 {
		equity = eq
	} else {
		equity = availableBalance // Fallback to available balance
	}

	// [CODE ENFORCED] Position Value Ratio Check: position_value <= equity × ratio
	adjustedPositionSize, wasCapped := at.enforcePositionValueRatio(decision.PositionSizeUSD, equity, decision.Symbol)
	if wasCapped {
		decision.PositionSizeUSD = adjustedPositionSize
	}

	// ⚠️ Auto-adjust position size if insufficient margin
	// Formula: totalRequired = positionSize/leverage + positionSize*0.001 + positionSize/leverage*0.01
	//        = positionSize * (1.01/leverage + 0.001)
	marginFactor := 1.01/float64(decision.Leverage) + 0.001
	maxAffordablePositionSize := availableBalance / marginFactor

	actualPositionSize := decision.PositionSizeUSD
	if actualPositionSize > maxAffordablePositionSize {
		// Use 98% of max to leave buffer for price fluctuation
		adjustedSize := maxAffordablePositionSize * 0.98
		logger.Infof("  ⚠️ Position size %.2f exceeds max affordable %.2f, auto-reducing to %.2f",
			actualPositionSize, maxAffordablePositionSize, adjustedSize)
		actualPositionSize = adjustedSize
		decision.PositionSizeUSD = actualPositionSize
	}

	// [CODE ENFORCED] Minimum position size check
	if err := at.enforceMinPositionSize(decision.PositionSizeUSD); err != nil {
		return err
	}

	// Calculate quantity with adjusted position size
	quantity := actualPositionSize / marketData.CurrentPrice
	actionRecord.Quantity = quantity
	actionRecord.Price = marketData.CurrentPrice

	// [CODE ENFORCED] Re-validate R:R with fresh market price before execution
	// Use 80% tolerance: market may drift during AI decision cycle
	if at.strategyEngine != nil {
		riskConfig := at.strategyEngine.GetRiskControlConfig()
		minRiskRewardRatio := riskConfig.MinRiskRewardRatio
		if hasDecisionTransform(decision, "reverse") {
			minRiskRewardRatio = at.getReverseMinRiskRewardRatio()
		}
		if err := kernel.ValidateRiskReward(decision.Action, marketData.CurrentPrice, decision.StopLoss, decision.TakeProfit, minRiskRewardRatio*0.8); err != nil {
			return fmt.Errorf("[pre-execution R:R check] %w", err)
		}
	}

	// Set margin mode
	if err := at.trader.SetMarginMode(decision.Symbol, at.config.IsCrossMargin); err != nil {
		logger.Infof("  ⚠️ Failed to set margin mode: %v", err)
		// Continue execution, doesn't affect trading
	}

	// Open position
	order, err := at.trader.OpenShort(decision.Symbol, quantity, decision.Leverage)
	if err != nil {
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}
	// Update entry price to actual fill price if available from exchange response
	if avgPrice, ok := order["avgPrice"].(float64); ok && avgPrice > 0 {
		actionRecord.Price = avgPrice
	}

	logger.Infof("  ✓ Position opened successfully, order ID: %v, quantity: %.4f", order["orderId"], quantity)

	if hasDecisionTransform(decision, "reverse") {
		at.markReversePosition(decision.Symbol, "short")
	}

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "open_short", quantity, marketData.CurrentPrice, decision.Leverage, 0)

	// Record position opening time
	posKey := decision.Symbol + "_short"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// Set stop loss and take profit
	if err := at.trader.SetStopLoss(decision.Symbol, "SHORT", quantity, decision.StopLoss); err != nil {
		logger.Infof("  ⚠ Failed to set stop loss: %v", err)
	}
	if err := at.trader.SetTakeProfit(decision.Symbol, "SHORT", quantity, decision.TakeProfit); err != nil {
		logger.Infof("  ⚠ Failed to set take profit: %v", err)
	}

	return nil
}

// executeCloseLongWithRecord executes close long position and records detailed information
func (at *AutoTrader) executeCloseLongWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  🔄 Close long: %s", decision.Symbol)

	// Get current price
	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice

	snapshot := at.loadClosePositionSnapshot(decision.Symbol, "long")
	if err := at.enforceMinHoldBeforeClose(decision.Symbol, snapshot); err != nil {
		return err
	}

	// Close position
	order, err := at.trader.CloseLong(decision.Symbol, 0) // 0 = close all
	if err != nil {
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "close_long", snapshot.quantity, marketData.CurrentPrice, 0, snapshot.entryPrice)
	at.SetLastCloseTime(snapshot.normalizedSymbol, time.Now())
	delete(at.positionFirstSeenTime, decision.Symbol+"_long")
	delete(at.positionFirstSeenTime, snapshot.normalizedSymbol+"_long")
	at.clearReversePosition(decision.Symbol, "long")
	at.clearReversePosition(snapshot.normalizedSymbol, "long")
	at.ClearPeakPnLCache(decision.Symbol, "long")
	at.ClearPeakPnLCache(snapshot.normalizedSymbol, "long")

	logger.Infof("  ✓ Position closed successfully")
	return nil
}

// executeCloseShortWithRecord executes close short position and records detailed information
func (at *AutoTrader) executeCloseShortWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  🔄 Close short: %s", decision.Symbol)

	// Get current price
	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice

	snapshot := at.loadClosePositionSnapshot(decision.Symbol, "short")
	if err := at.enforceMinHoldBeforeClose(decision.Symbol, snapshot); err != nil {
		return err
	}

	// Close position
	order, err := at.trader.CloseShort(decision.Symbol, 0) // 0 = close all
	if err != nil {
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "close_short", snapshot.quantity, marketData.CurrentPrice, 0, snapshot.entryPrice)
	at.SetLastCloseTime(snapshot.normalizedSymbol, time.Now())
	delete(at.positionFirstSeenTime, decision.Symbol+"_short")
	delete(at.positionFirstSeenTime, snapshot.normalizedSymbol+"_short")
	at.clearReversePosition(decision.Symbol, "short")
	at.clearReversePosition(snapshot.normalizedSymbol, "short")
	at.ClearPeakPnLCache(decision.Symbol, "short")
	at.ClearPeakPnLCache(snapshot.normalizedSymbol, "short")

	logger.Infof("  ✓ Position closed successfully")
	return nil
}
