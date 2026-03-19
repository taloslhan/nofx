package trader

import (
	"nofx/store"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// Helper: create a minimal AutoTrader for risk tests
// ============================================================================

func newTestAutoTrader(strategyConfig *store.StrategyConfig) *AutoTrader {
	at := &AutoTrader{
		config: AutoTraderConfig{
			StrategyConfig: strategyConfig,
		},
		peakPnLCache:       make(map[string]float64),
		peakPnLCacheMutex:  sync.RWMutex{},
		lastCloseTime:      make(map[string]time.Time),
		lastCloseTimeMutex: sync.RWMutex{},
	}
	return at
}

// ============================================================================
// Tests: UpdatePeakPnL / GetPeakPnLCache / ClearPeakPnLCache
// ============================================================================

func TestUpdatePeakPnL_FirstEntry(t *testing.T) {
	at := newTestAutoTrader(nil)
	at.UpdatePeakPnL("BTCUSDT", "long", 15.0)

	cache := at.GetPeakPnLCache()
	if cache["BTCUSDT_long"] != 15.0 {
		t.Errorf("expected peak 15.0, got %f", cache["BTCUSDT_long"])
	}
}

func TestUpdatePeakPnL_HigherValueUpdates(t *testing.T) {
	at := newTestAutoTrader(nil)
	at.UpdatePeakPnL("ETHUSDT", "short", 10.0)
	at.UpdatePeakPnL("ETHUSDT", "short", 20.0)

	cache := at.GetPeakPnLCache()
	if cache["ETHUSDT_short"] != 20.0 {
		t.Errorf("expected peak 20.0, got %f", cache["ETHUSDT_short"])
	}
}

func TestUpdatePeakPnL_LowerValueDoesNotUpdate(t *testing.T) {
	at := newTestAutoTrader(nil)
	at.UpdatePeakPnL("SOLUSDT", "long", 25.0)
	at.UpdatePeakPnL("SOLUSDT", "long", 10.0)

	cache := at.GetPeakPnLCache()
	if cache["SOLUSDT_long"] != 25.0 {
		t.Errorf("expected peak 25.0, got %f", cache["SOLUSDT_long"])
	}
}

func TestClearPeakPnLCache(t *testing.T) {
	at := newTestAutoTrader(nil)
	at.UpdatePeakPnL("BTCUSDT", "long", 30.0)
	at.ClearPeakPnLCache("BTCUSDT", "long")

	cache := at.GetPeakPnLCache()
	if _, exists := cache["BTCUSDT_long"]; exists {
		t.Error("expected cache entry to be cleared")
	}
}

func TestGetPeakPnLCache_ReturnsCopy(t *testing.T) {
	at := newTestAutoTrader(nil)
	at.UpdatePeakPnL("BTCUSDT", "long", 10.0)

	cache := at.GetPeakPnLCache()
	cache["BTCUSDT_long"] = 999.0

	original := at.GetPeakPnLCache()
	if original["BTCUSDT_long"] != 10.0 {
		t.Error("GetPeakPnLCache should return a copy, not a reference")
	}
}

func TestSetAndGetLastCloseTime(t *testing.T) {
	at := newTestAutoTrader(nil)
	now := time.Now().UTC().Truncate(time.Second)

	at.SetLastCloseTime("BTCUSDT", now)

	got := at.GetLastCloseTime("BTCUSDT")
	if !got.Equal(now) {
		t.Fatalf("expected %v, got %v", now, got)
	}
}

// ============================================================================
// Tests: isBTCETH
// ============================================================================

func TestIsBTCETH(t *testing.T) {
	tests := []struct {
		symbol string
		want   bool
	}{
		{"BTCUSDT", true},
		{"ETHUSDT", true},
		{"btcusdt", true},
		{"ethusdt", true},
		{"SOLUSDT", false},
		{"DOGEUSDT", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isBTCETH(tt.symbol); got != tt.want {
			t.Errorf("isBTCETH(%q) = %v, want %v", tt.symbol, got, tt.want)
		}
	}
}

// ============================================================================
// Tests: getSideFromAction
// ============================================================================

func TestGetSideFromAction(t *testing.T) {
	tests := []struct {
		action string
		want   string
	}{
		{"open_long", "BUY"},
		{"close_short", "BUY"},
		{"open_short", "SELL"},
		{"close_long", "SELL"},
		{"unknown", "BUY"},
	}
	for _, tt := range tests {
		if got := getSideFromAction(tt.action); got != tt.want {
			t.Errorf("getSideFromAction(%q) = %q, want %q", tt.action, got, tt.want)
		}
	}
}

// ============================================================================
// Tests: enforcePositionValueRatio
// ============================================================================

func newTestAutoTraderWithRisk(rc store.RiskControlConfig) *AutoTrader {
	return newTestAutoTrader(&store.StrategyConfig{
		RiskControl: rc,
	})
}

func TestEnforcePositionValueRatio_BTC_WithinLimit(t *testing.T) {
	at := newTestAutoTraderWithRisk(store.RiskControlConfig{
		BTCETHMaxPositionValueRatio: 5.0,
	})
	size, capped := at.enforcePositionValueRatio(4000, 1000, "BTCUSDT")
	if capped || size != 4000 {
		t.Errorf("expected (4000, false), got (%f, %v)", size, capped)
	}
}

func TestEnforcePositionValueRatio_BTC_ExceedsLimit(t *testing.T) {
	at := newTestAutoTraderWithRisk(store.RiskControlConfig{
		BTCETHMaxPositionValueRatio: 5.0,
	})
	size, capped := at.enforcePositionValueRatio(6000, 1000, "BTCUSDT")
	if !capped || size != 5000 {
		t.Errorf("expected (5000, true), got (%f, %v)", size, capped)
	}
}

func TestEnforcePositionValueRatio_Altcoin_Default(t *testing.T) {
	at := newTestAutoTraderWithRisk(store.RiskControlConfig{})
	// Default altcoin ratio is 1.0
	size, capped := at.enforcePositionValueRatio(1500, 1000, "SOLUSDT")
	if !capped || size != 1000 {
		t.Errorf("expected (1000, true), got (%f, %v)", size, capped)
	}
}

func TestEnforcePositionValueRatio_NilConfig(t *testing.T) {
	at := newTestAutoTrader(nil)
	size, capped := at.enforcePositionValueRatio(9999, 1000, "BTCUSDT")
	if capped || size != 9999 {
		t.Errorf("nil config should pass through, got (%f, %v)", size, capped)
	}
}

// ============================================================================
// Tests: enforceMinPositionSize
// ============================================================================

func TestEnforceMinPositionSize_AboveMin(t *testing.T) {
	at := newTestAutoTraderWithRisk(store.RiskControlConfig{MinPositionSize: 12})
	if err := at.enforceMinPositionSize(15); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestEnforceMinPositionSize_BelowMin(t *testing.T) {
	at := newTestAutoTraderWithRisk(store.RiskControlConfig{MinPositionSize: 12})
	if err := at.enforceMinPositionSize(5); err == nil {
		t.Error("expected error for position below minimum")
	}
}

func TestEnforceMinPositionSize_DefaultMin(t *testing.T) {
	at := newTestAutoTraderWithRisk(store.RiskControlConfig{})
	// Default min is 12
	if err := at.enforceMinPositionSize(5); err == nil {
		t.Error("expected error for position below default minimum (12)")
	}
}

func TestEnforceMinPositionSize_NilConfig(t *testing.T) {
	at := newTestAutoTrader(nil)
	if err := at.enforceMinPositionSize(1); err != nil {
		t.Errorf("nil config should pass, got %v", err)
	}
}

// ============================================================================
// Tests: enforceMaxPositions
// ============================================================================

func TestEnforceMaxPositions_BelowMax(t *testing.T) {
	at := newTestAutoTraderWithRisk(store.RiskControlConfig{MaxPositions: 3})
	if err := at.enforceMaxPositions(2); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestEnforceMaxPositions_AtMax(t *testing.T) {
	at := newTestAutoTraderWithRisk(store.RiskControlConfig{MaxPositions: 3})
	if err := at.enforceMaxPositions(3); err == nil {
		t.Error("expected error when at max positions")
	}
}

func TestEnforceMaxPositions_DefaultMax(t *testing.T) {
	at := newTestAutoTraderWithRisk(store.RiskControlConfig{})
	// Default max is 3
	if err := at.enforceMaxPositions(3); err == nil {
		t.Error("expected error at default max (3)")
	}
}

func TestEnforceMaxPositions_NilConfig(t *testing.T) {
	at := newTestAutoTrader(nil)
	if err := at.enforceMaxPositions(100); err != nil {
		t.Errorf("nil config should pass, got %v", err)
	}
}

func TestShouldBlockActiveClose_MinHoldSatisfied(t *testing.T) {
	blocked := shouldBlockActiveClose(2*time.Hour, 120, -5, -20)
	if blocked {
		t.Error("expected close to be allowed once min hold is satisfied")
	}
}

func TestShouldBlockActiveClose_NormalLossStillBlocked(t *testing.T) {
	blocked := shouldBlockActiveClose(30*time.Minute, 120, -8, -20)
	if !blocked {
		t.Error("expected close to be blocked before emergency loss threshold")
	}
}

func TestShouldBlockActiveClose_EmergencyLossAllowsBypass(t *testing.T) {
	blocked := shouldBlockActiveClose(30*time.Minute, 120, -25, -20)
	if blocked {
		t.Error("expected close to be allowed after emergency loss threshold")
	}
}

func TestNormalizeEmergencyCloseLossPct_DefaultFallback(t *testing.T) {
	if got := normalizeEmergencyCloseLossPct(0); got != -20 {
		t.Fatalf("expected fallback threshold -20, got %v", got)
	}
}
