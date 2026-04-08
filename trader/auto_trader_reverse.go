package trader

import (
	"fmt"
	"nofx/kernel"
	"nofx/logger"
	"nofx/market"
	"strings"
)

func normalizePositionSide(side string) string {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "long", "buy":
		return "long"
	case "short", "sell":
		return "short"
	default:
		return strings.ToLower(strings.TrimSpace(side))
	}
}

func reversePositionKey(symbol, side string) string {
	return market.Normalize(symbol) + "_" + normalizePositionSide(side)
}

func isOpenAction(action string) bool {
	return action == "open_long" || action == "open_short"
}

func reverseSideFromAction(action string) string {
	switch action {
	case "open_long":
		return "long"
	case "open_short":
		return "short"
	default:
		return ""
	}
}

func (at *AutoTrader) markReversePosition(symbol, side string) {
	if at == nil {
		return
	}

	at.reversePositionKeysMu.Lock()
	defer at.reversePositionKeysMu.Unlock()
	at.reversePositionKeys[reversePositionKey(symbol, side)] = true
}

func (at *AutoTrader) clearReversePosition(symbol, side string) {
	if at == nil {
		return
	}

	at.reversePositionKeysMu.Lock()
	defer at.reversePositionKeysMu.Unlock()
	delete(at.reversePositionKeys, reversePositionKey(symbol, side))
}

func (at *AutoTrader) isReversePosition(symbol, side string) bool {
	if at == nil {
		return false
	}

	at.reversePositionKeysMu.RLock()
	defer at.reversePositionKeysMu.RUnlock()
	return at.reversePositionKeys[reversePositionKey(symbol, side)]
}

func (at *AutoTrader) hasReversePositionForSymbol(symbol string) (bool, string) {
	if at == nil {
		return false, ""
	}

	normalizedSymbol := market.Normalize(symbol)
	at.reversePositionKeysMu.RLock()
	defer at.reversePositionKeysMu.RUnlock()

	for _, side := range []string{"long", "short"} {
		if at.reversePositionKeys[normalizedSymbol+"_"+side] {
			return true, side
		}
	}

	return false, ""
}

func (at *AutoTrader) syncReversePositionKeys(currentPositionKeys map[string]bool) {
	if at == nil {
		return
	}

	at.reversePositionKeysMu.Lock()
	defer at.reversePositionKeysMu.Unlock()

	for key := range at.reversePositionKeys {
		if !currentPositionKeys[key] {
			delete(at.reversePositionKeys, key)
		}
	}
}

func (at *AutoTrader) snapshotReversePositionState() map[string]string {
	states := make(map[string]string)
	if at == nil {
		return states
	}

	at.reversePositionKeysMu.RLock()
	defer at.reversePositionKeysMu.RUnlock()

	for key := range at.reversePositionKeys {
		symbol, side, ok := strings.Cut(key, "_")
		if ok {
			states[symbol] = side
		}
	}

	return states
}

func (at *AutoTrader) planReverseShadowDecisions(decisions []kernel.Decision) []kernel.Decision {
	if at == nil || len(decisions) == 0 {
		return decisions
	}

	planned := make([]kernel.Decision, 0, len(decisions)+2)
	reverseStates := at.snapshotReversePositionState()

	for _, decision := range decisions {
		if !hasDecisionTransform(&decision, "reverse") || !isOpenAction(decision.Action) {
			planned = append(planned, decision)
			continue
		}

		normalizedSymbol := market.Normalize(decision.Symbol)
		targetSide := reverseSideFromAction(decision.Action)
		existingSide := reverseStates[normalizedSymbol]

		if existingSide == "" {
			planned = append(planned, decision)
			reverseStates[normalizedSymbol] = targetSide
			continue
		}

		if existingSide == targetSide {
			logger.Infof("⏭️ [REVERSE] Shadow %s already exists for %s, skipping duplicate open", targetSide, normalizedSymbol)
			continue
		}

		logger.Infof("🔄 [REVERSE] AI direction flipped for %s: close shadow %s before open %s",
			normalizedSymbol, existingSide, targetSide)

		planned = append(planned, kernel.Decision{
			Symbol:    normalizedSymbol,
			Action:    "close_" + existingSide,
			Reasoning: fmt.Sprintf("[REVERSE] Close shadow %s before switching to %s", existingSide, targetSide),
		})
		decision.DependsOnSymbolClose = normalizedSymbol
		planned = append(planned, decision)
		reverseStates[normalizedSymbol] = targetSide
	}

	return planned
}

func (at *AutoTrader) reconcileReversePositionState(symbol, side string, dbPosReverse bool) {
	if at == nil {
		return
	}

	if dbPosReverse {
		if !at.isReversePosition(symbol, side) {
			at.markReversePosition(symbol, side)
		}
		return
	}

	if !at.isReversePosition(symbol, side) || at.store == nil {
		return
	}

	if err := at.store.Position().SetOpenPositionReverseFlag(at.id, symbol, side, true); err != nil {
		logger.Infof("⚠️ [REVERSE] Failed to persist reverse flag for %s %s: %v", symbol, side, err)
	}
}

func (at *AutoTrader) recoverReversePositionsFromStore() {
	if at == nil || at.store == nil {
		return
	}

	positions, err := at.store.Position().GetOpenReversePositions(at.id)
	if err != nil {
		logger.Infof("⚠️ [REVERSE] Failed to recover reverse positions: %v", err)
		return
	}

	for _, pos := range positions {
		at.markReversePosition(pos.Symbol, pos.Side)
	}

	if len(positions) > 0 {
		logger.Infof("🔄 [REVERSE] Recovered %d reverse positions from store", len(positions))
	}
}
