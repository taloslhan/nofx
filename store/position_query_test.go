package store

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupPositionQueryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := NewTraderStore(db).initTables(); err != nil {
		t.Fatalf("failed to initialize trader tables: %v", err)
	}
	if err := NewPositionStore(db).InitTables(); err != nil {
		t.Fatalf("failed to initialize position tables: %v", err)
	}

	return db
}

func TestPositionBuilderResolvesLeverageFromTraderConfig(t *testing.T) {
	db := setupPositionQueryTestDB(t)

	traderStore := NewTraderStore(db)
	if err := traderStore.Create(&Trader{
		ID:              "trader-1",
		UserID:          "user-1",
		Name:            "test",
		AIModelID:       "model-1",
		ExchangeID:      "exchange-1",
		InitialBalance:  1000,
		BTCETHLeverage:  10,
		AltcoinLeverage: 5,
	}); err != nil {
		t.Fatalf("failed to create trader: %v", err)
	}

	positionStore := NewPositionStore(db)
	builder := NewPositionBuilder(positionStore)

	if err := builder.ProcessTrade(
		"trader-1", "exchange-1", "binance",
		"BTCUSDT", "LONG", "open_long",
		0.1, 50000, 1, 0, 0,
		time.Now().UTC().UnixMilli(), "order-1",
	); err != nil {
		t.Fatalf("failed to process trade: %v", err)
	}

	position, err := positionStore.GetOpenPositionBySymbol("trader-1", "BTCUSDT", "LONG")
	if err != nil {
		t.Fatalf("failed to load position: %v", err)
	}
	if position == nil {
		t.Fatal("expected open position")
	}
	if position.Leverage != 10 {
		t.Fatalf("expected leverage 10, got %d", position.Leverage)
	}
}

func TestGetFullStatsAppliesResetTimeAndNetPnL(t *testing.T) {
	db := setupPositionQueryTestDB(t)

	resetTime := time.Now().UTC().UnixMilli()
	traderStore := NewTraderStore(db)
	if err := traderStore.Create(&Trader{
		ID:             "trader-2",
		UserID:         "user-2",
		Name:           "stats",
		AIModelID:      "model-2",
		ExchangeID:     "exchange-2",
		InitialBalance: 1000,
		StatsResetTime: resetTime,
	}); err != nil {
		t.Fatalf("failed to create trader: %v", err)
	}

	positionStore := NewPositionStore(db)
	positions := []*TraderPosition{
		{
			TraderID:    "trader-2",
			ExchangeID:  "exchange-2",
			Symbol:      "BTCUSDT",
			Side:        "LONG",
			Quantity:    0.1,
			EntryPrice:  50000,
			EntryTime:   resetTime - 20000,
			ExitPrice:   51000,
			ExitTime:    resetTime - 10000,
			RealizedPnL: 100,
			Fee:         5,
			Status:      "CLOSED",
		},
		{
			TraderID:    "trader-2",
			ExchangeID:  "exchange-2",
			Symbol:      "ETHUSDT",
			Side:        "SHORT",
			Quantity:    1,
			EntryPrice:  2500,
			EntryTime:   resetTime + 1000,
			ExitPrice:   2480,
			ExitTime:    resetTime + 5000,
			RealizedPnL: 20,
			Fee:         3,
			Status:      "CLOSED",
		},
	}
	for _, position := range positions {
		if err := db.Create(position).Error; err != nil {
			t.Fatalf("failed to create position: %v", err)
		}
	}

	stats, err := positionStore.GetFullStats("trader-2")
	if err != nil {
		t.Fatalf("failed to get full stats: %v", err)
	}

	if stats.TotalTrades != 1 {
		t.Fatalf("expected 1 trade after reset, got %d", stats.TotalTrades)
	}
	if stats.TotalPnL != 20 {
		t.Fatalf("expected total_pnl 20, got %.2f", stats.TotalPnL)
	}
	if stats.GrossRealizedPnL != 20 {
		t.Fatalf("expected gross_realized_pnl 20, got %.2f", stats.GrossRealizedPnL)
	}
	if stats.NetPnL != 17 {
		t.Fatalf("expected net_pnl 17, got %.2f", stats.NetPnL)
	}

	closed, err := positionStore.GetClosedPositions("trader-2", 10)
	if err != nil {
		t.Fatalf("failed to get closed positions: %v", err)
	}
	if len(closed) != 1 {
		t.Fatalf("expected 1 closed position after reset, got %d", len(closed))
	}
	if closed[0].Symbol != "ETHUSDT" {
		t.Fatalf("expected ETHUSDT after reset, got %s", closed[0].Symbol)
	}
}
