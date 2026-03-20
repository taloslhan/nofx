package trader

import (
	"nofx/kernel"
	"nofx/store"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type reverseTestTrader struct {
	balance   map[string]interface{}
	positions []map[string]interface{}
}

func (t *reverseTestTrader) GetBalance() (map[string]interface{}, error) { return t.balance, nil }
func (t *reverseTestTrader) GetPositions() ([]map[string]interface{}, error) {
	return t.positions, nil
}
func (t *reverseTestTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return nil, nil
}
func (t *reverseTestTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return nil, nil
}
func (t *reverseTestTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	return nil, nil
}
func (t *reverseTestTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	return nil, nil
}
func (t *reverseTestTrader) SetLeverage(symbol string, leverage int) error { return nil }
func (t *reverseTestTrader) SetMarginMode(symbol string, isCrossMargin bool) error {
	return nil
}
func (t *reverseTestTrader) GetMarketPrice(symbol string) (float64, error) { return 0, nil }
func (t *reverseTestTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	return nil
}
func (t *reverseTestTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	return nil
}
func (t *reverseTestTrader) CancelStopLossOrders(symbol string) error { return nil }
func (t *reverseTestTrader) CancelTakeProfitOrders(symbol string) error {
	return nil
}
func (t *reverseTestTrader) CancelAllOrders(symbol string) error  { return nil }
func (t *reverseTestTrader) CancelStopOrders(symbol string) error { return nil }
func (t *reverseTestTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	return "", nil
}
func (t *reverseTestTrader) GetOrderStatus(symbol string, orderID string) (map[string]interface{}, error) {
	return nil, nil
}
func (t *reverseTestTrader) GetClosedPnL(startTime time.Time, limit int) ([]ClosedPnLRecord, error) {
	return nil, nil
}
func (t *reverseTestTrader) GetOpenOrders(symbol string) ([]OpenOrder, error) { return nil, nil }

func newReverseTestAutoTrader(strategyConfig *store.StrategyConfig, tr Trader) *AutoTrader {
	return &AutoTrader{
		name:                  "reverse-test",
		config:                AutoTraderConfig{StrategyConfig: strategyConfig},
		trader:                tr,
		strategyEngine:        kernel.NewStrategyEngine(strategyConfig),
		initialBalance:        1000,
		startTime:             time.Now().Add(-10 * time.Minute),
		positionFirstSeenTime: make(map[string]int64),
		lastCloseTime:         make(map[string]time.Time),
		lastCloseTimeMutex:    sync.RWMutex{},
		reversePositionKeys:   make(map[string]bool),
		reversePositionKeysMu: sync.RWMutex{},
		peakPnLCache:          make(map[string]float64),
		peakPnLCacheMutex:     sync.RWMutex{},
	}
}

func TestReversePositionTracking(t *testing.T) {
	at := newTestAutoTrader(nil)

	at.markReversePosition("BTC", "SELL")
	if !at.isReversePosition("BTCUSDT", "short") {
		t.Fatalf("expected reverse position to be tracked")
	}

	exists, side := at.hasReversePositionForSymbol("BTCUSDT")
	if !exists || side != "short" {
		t.Fatalf("expected short reverse position, got exists=%v side=%q", exists, side)
	}

	at.clearReversePosition("BTCUSDT", "short")
	if at.isReversePosition("BTCUSDT", "short") {
		t.Fatalf("expected reverse position to be cleared")
	}
}

func TestSyncReversePositionKeysRemovesClosedEntries(t *testing.T) {
	at := newTestAutoTrader(nil)
	at.markReversePosition("BTCUSDT", "short")
	at.markReversePosition("ETHUSDT", "long")

	at.syncReversePositionKeys(map[string]bool{
		reversePositionKey("ETHUSDT", "long"): true,
	})

	if at.isReversePosition("BTCUSDT", "short") {
		t.Fatalf("expected stale reverse key to be removed")
	}
	if !at.isReversePosition("ETHUSDT", "long") {
		t.Fatalf("expected active reverse key to remain")
	}
}

func TestPlanReverseShadowDecisionsSkipsDuplicateOpen(t *testing.T) {
	at := newTestAutoTrader(nil)
	at.markReversePosition("BTCUSDT", "short")

	planned := at.planReverseShadowDecisions([]kernel.Decision{
		{
			Symbol:           "BTCUSDT",
			Action:           "open_short",
			TransformApplied: []string{"reverse"},
		},
	})

	if len(planned) != 0 {
		t.Fatalf("expected duplicate reverse open to be skipped, got %d decisions", len(planned))
	}
}

func TestPlanReverseShadowDecisionsInsertsCloseOnFlip(t *testing.T) {
	at := newTestAutoTrader(nil)
	at.markReversePosition("BTCUSDT", "short")

	planned := at.planReverseShadowDecisions([]kernel.Decision{
		{
			Symbol:           "BTCUSDT",
			Action:           "open_long",
			TransformApplied: []string{"reverse"},
		},
	})

	if len(planned) != 2 {
		t.Fatalf("expected close+open plan, got %d decisions", len(planned))
	}
	if planned[0].Action != "close_short" {
		t.Fatalf("expected first action close_short, got %s", planned[0].Action)
	}
	if planned[1].Action != "open_long" {
		t.Fatalf("expected second action open_long, got %s", planned[1].Action)
	}
}

func TestBuildTradingContextHidesReversePositionsButKeepsMargin(t *testing.T) {
	strategyConfig := &store.StrategyConfig{
		CoinSource: store.CoinSourceConfig{
			SourceType:  "static",
			StaticCoins: []string{"BTCUSDT"},
		},
	}

	tr := &reverseTestTrader{
		balance: map[string]interface{}{
			"totalWalletBalance":    1000.0,
			"totalUnrealizedProfit": 15.0,
			"availableBalance":      800.0,
			"totalEquity":           1015.0,
		},
		positions: []map[string]interface{}{
			{
				"symbol":           "BTCUSDT",
				"side":             "short",
				"entryPrice":       100.0,
				"markPrice":        110.0,
				"positionAmt":      2.0,
				"unRealizedProfit": -5.0,
				"liquidationPrice": 150.0,
				"leverage":         10.0,
			},
			{
				"symbol":           "ETHUSDT",
				"side":             "long",
				"entryPrice":       50.0,
				"markPrice":        55.0,
				"positionAmt":      1.0,
				"unRealizedProfit": 20.0,
				"liquidationPrice": 20.0,
				"leverage":         5.0,
			},
		},
	}

	at := newReverseTestAutoTrader(strategyConfig, tr)
	at.markReversePosition("BTCUSDT", "short")

	ctx, err := at.buildTradingContext()
	if err != nil {
		t.Fatalf("buildTradingContext() error = %v", err)
	}

	if len(ctx.Positions) != 1 {
		t.Fatalf("expected 1 visible position, got %d", len(ctx.Positions))
	}
	if ctx.Positions[0].Symbol != "ETHUSDT" {
		t.Fatalf("expected ETHUSDT to remain visible, got %s", ctx.Positions[0].Symbol)
	}

	expectedMarginUsed := 33.0
	if ctx.Account.MarginUsed != expectedMarginUsed {
		t.Fatalf("expected margin used %.2f, got %.2f", expectedMarginUsed, ctx.Account.MarginUsed)
	}

	if ctx.Account.PositionCount != 1 {
		t.Fatalf("expected visible position count 1, got %d", ctx.Account.PositionCount)
	}
}

func TestBuildTradingContextRecoversReverseFlagFromStore(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := store.NewPositionStore(db).InitTables(); err != nil {
		t.Fatalf("failed to initialize position tables: %v", err)
	}

	st, err := store.NewFromGorm(db)
	if err != nil {
		t.Fatalf("failed to create store wrapper: %v", err)
	}

	nowMs := time.Now().UTC().UnixMilli()
	if err := st.Position().CreateOpenPosition(&store.TraderPosition{
		TraderID:   "trader-1",
		ExchangeID: "exchange-1",
		Symbol:     "BTCUSDT",
		Side:       "SHORT",
		Quantity:   2,
		EntryPrice: 100,
		EntryTime:  nowMs,
		Leverage:   10,
		IsReverse:  true,
		Status:     "OPEN",
		CreatedAt:  nowMs,
		UpdatedAt:  nowMs,
	}); err != nil {
		t.Fatalf("failed to create reverse position: %v", err)
	}

	strategyConfig := &store.StrategyConfig{
		CoinSource: store.CoinSourceConfig{
			SourceType:  "static",
			StaticCoins: []string{"BTCUSDT"},
		},
	}

	tr := &reverseTestTrader{
		balance: map[string]interface{}{
			"totalWalletBalance":    1000.0,
			"totalUnrealizedProfit": -5.0,
			"availableBalance":      900.0,
			"totalEquity":           995.0,
		},
		positions: []map[string]interface{}{
			{
				"symbol":           "BTCUSDT",
				"side":             "short",
				"entryPrice":       100.0,
				"markPrice":        110.0,
				"positionAmt":      2.0,
				"unRealizedProfit": -5.0,
				"liquidationPrice": 150.0,
				"leverage":         10.0,
			},
		},
	}

	at := newReverseTestAutoTrader(strategyConfig, tr)
	at.id = "trader-1"
	at.store = st

	ctx, err := at.buildTradingContext()
	if err != nil {
		t.Fatalf("buildTradingContext() error = %v", err)
	}

	if len(ctx.Positions) != 0 {
		t.Fatalf("expected reverse position to be hidden after store recovery, got %d positions", len(ctx.Positions))
	}
	if !at.isReversePosition("BTCUSDT", "short") {
		t.Fatalf("expected reverse flag to be recovered into memory")
	}
}
