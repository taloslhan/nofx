package store

import (
	"math"
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

func TestPositionStatsUseNetPnLForDerivedMetrics(t *testing.T) {
	db := setupPositionQueryTestDB(t)

	traderStore := NewTraderStore(db)
	if err := traderStore.Create(&Trader{
		ID:             "trader-3",
		UserID:         "user-3",
		Name:           "net-metrics",
		AIModelID:      "model-3",
		ExchangeID:     "exchange-3",
		InitialBalance: 1000,
	}); err != nil {
		t.Fatalf("failed to create trader: %v", err)
	}

	positionStore := NewPositionStore(db)
	baseTime := time.Now().UTC().UnixMilli()
	positions := []*TraderPosition{
		{
			TraderID:    "trader-3",
			ExchangeID:  "exchange-3",
			Symbol:      "BTCUSDT",
			Side:        "LONG",
			Quantity:    0.1,
			EntryPrice:  50000,
			EntryTime:   baseTime,
			ExitPrice:   50100,
			ExitTime:    baseTime + 60000,
			RealizedPnL: 5,
			Fee:         7,
			Status:      "CLOSED",
		},
		{
			TraderID:    "trader-3",
			ExchangeID:  "exchange-3",
			Symbol:      "BTCUSDT",
			Side:        "LONG",
			Quantity:    0.2,
			EntryPrice:  50000,
			EntryTime:   baseTime + 120000,
			ExitPrice:   50200,
			ExitTime:    baseTime + 180000,
			RealizedPnL: 4,
			Fee:         1,
			Status:      "CLOSED",
		},
		{
			TraderID:    "trader-3",
			ExchangeID:  "exchange-3",
			Symbol:      "ETHUSDT",
			Side:        "SHORT",
			Quantity:    1,
			EntryPrice:  2500,
			EntryTime:   baseTime + 240000,
			ExitPrice:   2520,
			ExitTime:    baseTime + 300000,
			RealizedPnL: -2,
			Fee:         1,
			Status:      "CLOSED",
		},
	}
	for _, position := range positions {
		if err := db.Create(position).Error; err != nil {
			t.Fatalf("failed to create position: %v", err)
		}
	}

	statsMap, err := positionStore.GetPositionStats("trader-3")
	if err != nil {
		t.Fatalf("failed to get position stats: %v", err)
	}
	if wins, ok := statsMap["win_trades"].(int); !ok || wins != 1 {
		t.Fatalf("expected win_trades 1 from net pnl, got %#v", statsMap["win_trades"])
	}
	if winRate, ok := statsMap["win_rate"].(float64); !ok || math.Abs(winRate-33.3333333333) > 0.0001 {
		t.Fatalf("expected win_rate 33.33, got %#v", statsMap["win_rate"])
	}

	fullStats, err := positionStore.GetFullStats("trader-3")
	if err != nil {
		t.Fatalf("failed to get full stats: %v", err)
	}
	if fullStats.WinTrades != 1 || fullStats.LossTrades != 2 {
		t.Fatalf("expected 1 win / 2 losses from net pnl, got %d / %d", fullStats.WinTrades, fullStats.LossTrades)
	}
	if math.Abs(fullStats.ProfitFactor-0.6) > 0.0001 {
		t.Fatalf("expected profit factor 0.60, got %.4f", fullStats.ProfitFactor)
	}
	if math.Abs(fullStats.AvgWin-3) > 0.0001 {
		t.Fatalf("expected avg win 3, got %.4f", fullStats.AvgWin)
	}
	if math.Abs(fullStats.AvgLoss-2.5) > 0.0001 {
		t.Fatalf("expected avg loss 2.5, got %.4f", fullStats.AvgLoss)
	}
	if math.Abs(fullStats.GrossRealizedPnL-7) > 0.0001 || math.Abs(fullStats.TotalFee-9) > 0.0001 || math.Abs(fullStats.NetPnL+2) > 0.0001 {
		t.Fatalf("expected gross 7 fee 9 net -2, got gross %.4f fee %.4f net %.4f", fullStats.GrossRealizedPnL, fullStats.TotalFee, fullStats.NetPnL)
	}

	expectedPnls := []float64{-2, 3, -3}
	if math.Abs(fullStats.SharpeRatio-calculateSharpeRatioFromPnls(expectedPnls)) > 0.0001 {
		t.Fatalf("expected sharpe ratio to use net pnls, got %.6f", fullStats.SharpeRatio)
	}
	if math.Abs(fullStats.MaxDrawdownPct-calculateMaxDrawdownFromPnls(expectedPnls)) > 0.0001 {
		t.Fatalf("expected max drawdown to use net pnls, got %.6f", fullStats.MaxDrawdownPct)
	}

	symbolStats, err := positionStore.GetSymbolStats("trader-3", 10)
	if err != nil {
		t.Fatalf("failed to get symbol stats: %v", err)
	}
	if len(symbolStats) != 2 {
		t.Fatalf("expected 2 symbol stats, got %d", len(symbolStats))
	}
	if symbolStats[0].Symbol != "BTCUSDT" {
		t.Fatalf("expected BTCUSDT to sort first by net pnl, got %s", symbolStats[0].Symbol)
	}
	if math.Abs(symbolStats[0].NetPnL-1) > 0.0001 || math.Abs(symbolStats[0].TotalFee-8) > 0.0001 || math.Abs(symbolStats[0].AvgPnL-0.5) > 0.0001 {
		t.Fatalf("expected BTC stats to use net pnl, got net %.4f fee %.4f avg %.4f", symbolStats[0].NetPnL, symbolStats[0].TotalFee, symbolStats[0].AvgPnL)
	}
	if math.Abs(symbolStats[0].WinRate-50) > 0.0001 {
		t.Fatalf("expected BTC win rate 50, got %.4f", symbolStats[0].WinRate)
	}

	directionStats, err := positionStore.GetDirectionStats("trader-3")
	if err != nil {
		t.Fatalf("failed to get direction stats: %v", err)
	}
	if len(directionStats) != 2 {
		t.Fatalf("expected 2 direction stats, got %d", len(directionStats))
	}
	for _, stat := range directionStats {
		switch stat.Side {
		case "LONG":
			if math.Abs(stat.NetPnL-1) > 0.0001 || math.Abs(stat.TotalFee-8) > 0.0001 || math.Abs(stat.AvgPnL-0.5) > 0.0001 || math.Abs(stat.WinRate-50) > 0.0001 {
				t.Fatalf("expected LONG stats to use net pnl, got %+v", stat)
			}
		case "SHORT":
			if math.Abs(stat.NetPnL+3) > 0.0001 || math.Abs(stat.TotalFee-1) > 0.0001 || math.Abs(stat.AvgPnL+3) > 0.0001 || math.Abs(stat.WinRate) > 0.0001 {
				t.Fatalf("expected SHORT stats to use net pnl, got %+v", stat)
			}
		default:
			t.Fatalf("unexpected side %s", stat.Side)
		}
	}

	historySummary, err := positionStore.GetHistorySummary("trader-3")
	if err != nil {
		t.Fatalf("failed to get history summary: %v", err)
	}
	if math.Abs(historySummary.TotalPnL+2) > 0.0001 || math.Abs(historySummary.AvgTradeReturn+0.6666666667) > 0.0001 {
		t.Fatalf("expected history summary totals to use net pnl, got total %.4f avg %.4f", historySummary.TotalPnL, historySummary.AvgTradeReturn)
	}
	if math.Abs(historySummary.LongPnL-1) > 0.0001 || math.Abs(historySummary.ShortPnL+3) > 0.0001 {
		t.Fatalf("expected direction pnl in history summary to use net pnl, got long %.4f short %.4f", historySummary.LongPnL, historySummary.ShortPnL)
	}
	if math.Abs(historySummary.RecentPnL+2) > 0.0001 || math.Abs(historySummary.RecentWinRate-33.3333333333) > 0.0001 {
		t.Fatalf("expected recent metrics to use net pnl, got pnl %.4f win rate %.4f", historySummary.RecentPnL, historySummary.RecentWinRate)
	}
	if len(historySummary.BestSymbols) != 1 || historySummary.BestSymbols[0].Symbol != "BTCUSDT" {
		t.Fatalf("expected BTCUSDT as only profitable symbol after fees, got %+v", historySummary.BestSymbols)
	}
	if len(historySummary.WorstSymbols) != 1 || historySummary.WorstSymbols[0].Symbol != "ETHUSDT" {
		t.Fatalf("expected ETHUSDT as losing symbol after fees, got %+v", historySummary.WorstSymbols)
	}
}

func TestPositionStoreReverseFlagPersistence(t *testing.T) {
	db := setupPositionQueryTestDB(t)
	positionStore := NewPositionStore(db)

	nowMs := time.Now().UTC().UnixMilli()
	pos := &TraderPosition{
		TraderID:   "trader-reverse",
		ExchangeID: "exchange-1",
		Symbol:     "BTCUSDT",
		Side:       "LONG",
		Quantity:   1,
		EntryPrice: 50000,
		EntryTime:  nowMs,
		Status:     "OPEN",
		CreatedAt:  nowMs,
		UpdatedAt:  nowMs,
	}
	if err := positionStore.CreateOpenPosition(pos); err != nil {
		t.Fatalf("failed to create position: %v", err)
	}

	if err := positionStore.SetOpenPositionReverseFlag("trader-reverse", "BTCUSDT", "long", true); err != nil {
		t.Fatalf("failed to set reverse flag: %v", err)
	}

	loaded, err := positionStore.GetOpenPositionBySymbol("trader-reverse", "BTCUSDT", "long")
	if err != nil {
		t.Fatalf("failed to reload position: %v", err)
	}
	if loaded == nil || !loaded.IsReverse {
		t.Fatalf("expected open position to be marked reverse")
	}

	reversePositions, err := positionStore.GetOpenReversePositions("trader-reverse")
	if err != nil {
		t.Fatalf("failed to query open reverse positions: %v", err)
	}
	if len(reversePositions) != 1 {
		t.Fatalf("expected 1 reverse position, got %d", len(reversePositions))
	}
}
