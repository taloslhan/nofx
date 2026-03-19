package trader

import (
	"fmt"
	"nofx/logger"
	"strings"
	"time"
)

// startDrawdownMonitor starts drawdown monitoring
func (at *AutoTrader) startDrawdownMonitor() {
	at.monitorWg.Add(1)
	go func() {
		defer at.monitorWg.Done()

		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		logger.Info("📊 Started position drawdown monitoring (check every minute)")

		for {
			select {
			case <-ticker.C:
				at.checkPositionDrawdown()
			case <-at.stopMonitorCh:
				logger.Info("⏹ Stopped position drawdown monitoring")
				return
			}
		}
	}()
}

// checkPositionDrawdown checks position drawdown situation (monitoring only, no auto-close)
func (at *AutoTrader) checkPositionDrawdown() {
	positions, err := at.trader.GetPositions()
	if err != nil {
		logger.Infof("❌ Drawdown monitoring: failed to get positions: %v", err)
		return
	}

	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)

		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}

		var currentPnLPct float64
		if side == "long" {
			currentPnLPct = ((markPrice - entryPrice) / entryPrice) * float64(leverage) * 100
		} else {
			currentPnLPct = ((entryPrice - markPrice) / entryPrice) * float64(leverage) * 100
		}

		// Update peak PnL cache (for AI reference only)
		at.UpdatePeakPnL(symbol, side, currentPnLPct)

		if currentPnLPct > 5.0 {
			posKey := symbol + "_" + side
			at.peakPnLCacheMutex.RLock()
			peakPnLPct := at.peakPnLCache[posKey]
			at.peakPnLCacheMutex.RUnlock()

			var drawdownPct float64
			if peakPnLPct > 0 && currentPnLPct < peakPnLPct {
				drawdownPct = ((peakPnLPct - currentPnLPct) / peakPnLPct) * 100
			}

			logger.Infof("📊 Drawdown monitoring: %s %s | Profit: %.2f%% | Peak: %.2f%% | Drawdown: %.2f%%",
				symbol, side, currentPnLPct, peakPnLPct, drawdownPct)
		}
	}
}

// GetPeakPnLCache gets peak profit cache
func (at *AutoTrader) GetPeakPnLCache() map[string]float64 {
	at.peakPnLCacheMutex.RLock()
	defer at.peakPnLCacheMutex.RUnlock()
	cache := make(map[string]float64)
	for k, v := range at.peakPnLCache {
		cache[k] = v
	}
	return cache
}

// UpdatePeakPnL updates peak profit cache
func (at *AutoTrader) UpdatePeakPnL(symbol, side string, currentPnLPct float64) {
	at.peakPnLCacheMutex.Lock()
	defer at.peakPnLCacheMutex.Unlock()
	posKey := symbol + "_" + side
	if peak, exists := at.peakPnLCache[posKey]; exists {
		if currentPnLPct > peak {
			at.peakPnLCache[posKey] = currentPnLPct
		}
	} else {
		at.peakPnLCache[posKey] = currentPnLPct
	}
}

// ClearPeakPnLCache clears peak cache for specified position
func (at *AutoTrader) ClearPeakPnLCache(symbol, side string) {
	at.peakPnLCacheMutex.Lock()
	defer at.peakPnLCacheMutex.Unlock()
	posKey := symbol + "_" + side
	delete(at.peakPnLCache, posKey)
}

// SetLastCloseTime records the latest successful close time for a symbol.
func (at *AutoTrader) SetLastCloseTime(symbol string, closeTime time.Time) {
	at.lastCloseTimeMutex.Lock()
	defer at.lastCloseTimeMutex.Unlock()
	at.lastCloseTime[symbol] = closeTime.UTC()
}

// GetLastCloseTime returns the latest successful close time for a symbol.
func (at *AutoTrader) GetLastCloseTime(symbol string) time.Time {
	at.lastCloseTimeMutex.RLock()
	defer at.lastCloseTimeMutex.RUnlock()
	return at.lastCloseTime[symbol]
}

func normalizeEmergencyCloseLossPct(threshold float64) float64 {
	if threshold >= 0 {
		return -20.0
	}
	return threshold
}

func shouldBlockActiveClose(holdDuration time.Duration, minHoldMinutes int, currentPnLPct float64, emergencyLossPct float64) bool {
	if minHoldMinutes <= 0 {
		return false
	}
	if holdDuration >= time.Duration(minHoldMinutes)*time.Minute {
		return false
	}
	return currentPnLPct > normalizeEmergencyCloseLossPct(emergencyLossPct)
}

// ============================================================================
// Risk Control Helpers
// ============================================================================

// isBTCETH checks if a symbol is BTC or ETH
func isBTCETH(symbol string) bool {
	symbol = strings.ToUpper(symbol)
	return strings.HasPrefix(symbol, "BTC") || strings.HasPrefix(symbol, "ETH")
}

// enforcePositionValueRatio checks and enforces position value ratio limits (CODE ENFORCED)
// Returns the adjusted position size (capped if necessary) and whether the position was capped
// positionSizeUSD: the original position size in USD
// equity: the account equity
// symbol: the trading symbol
func (at *AutoTrader) enforcePositionValueRatio(positionSizeUSD float64, equity float64, symbol string) (float64, bool) {
	if at.config.StrategyConfig == nil {
		return positionSizeUSD, false
	}

	riskControl := at.config.StrategyConfig.RiskControl

	// Get the appropriate position value ratio limit
	var maxPositionValueRatio float64
	if isBTCETH(symbol) {
		maxPositionValueRatio = riskControl.BTCETHMaxPositionValueRatio
		if maxPositionValueRatio <= 0 {
			maxPositionValueRatio = 5.0 // Default: 5x for BTC/ETH
		}
	} else {
		maxPositionValueRatio = riskControl.AltcoinMaxPositionValueRatio
		if maxPositionValueRatio <= 0 {
			maxPositionValueRatio = 1.0 // Default: 1x for altcoins
		}
	}

	// Calculate max allowed position value = equity × ratio
	maxPositionValue := equity * maxPositionValueRatio

	// Check if position size exceeds limit
	if positionSizeUSD > maxPositionValue {
		logger.Infof("  ⚠️ [RISK CONTROL] Position %.2f USDT exceeds limit (equity %.2f × %.1fx = %.2f USDT max for %s), capping",
			positionSizeUSD, equity, maxPositionValueRatio, maxPositionValue, symbol)
		return maxPositionValue, true
	}

	return positionSizeUSD, false
}

// enforceMinPositionSize checks minimum position size (CODE ENFORCED)
func (at *AutoTrader) enforceMinPositionSize(positionSizeUSD float64) error {
	if at.config.StrategyConfig == nil {
		return nil
	}

	minSize := at.config.StrategyConfig.RiskControl.MinPositionSize
	if minSize <= 0 {
		minSize = 12 // Default: 12 USDT
	}

	if positionSizeUSD < minSize {
		return fmt.Errorf("❌ [RISK CONTROL] Position %.2f USDT below minimum (%.2f USDT)", positionSizeUSD, minSize)
	}
	return nil
}

// enforceMaxPositions checks maximum positions count (CODE ENFORCED)
func (at *AutoTrader) enforceMaxPositions(currentPositionCount int) error {
	if at.config.StrategyConfig == nil {
		return nil
	}

	maxPositions := at.config.StrategyConfig.RiskControl.MaxPositions
	if maxPositions <= 0 {
		maxPositions = 3 // Default: 3 positions
	}

	if currentPositionCount >= maxPositions {
		return fmt.Errorf("❌ [RISK CONTROL] Already at max positions (%d/%d)", currentPositionCount, maxPositions)
	}
	return nil
}

// getSideFromAction converts order action to side (BUY/SELL)
func getSideFromAction(action string) string {
	switch action {
	case "open_long", "close_short":
		return "BUY"
	case "open_short", "close_long":
		return "SELL"
	default:
		return "BUY"
	}
}
