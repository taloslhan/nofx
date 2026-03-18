package store

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestTraderStoreEnsureColumnsAddsFallbackAIModelID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE traders (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			ai_model_id TEXT NOT NULL,
			exchange_id TEXT NOT NULL,
			strategy_id TEXT DEFAULT '',
			initial_balance REAL NOT NULL,
			scan_interval_minutes INTEGER DEFAULT 3,
			is_running BOOLEAN DEFAULT FALSE,
			is_cross_margin BOOLEAN DEFAULT TRUE,
			show_in_competition BOOLEAN DEFAULT TRUE,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("failed to create legacy traders table: %v", err)
	}

	traderStore := NewTraderStore(db)
	if err := traderStore.ensureColumns(); err != nil {
		t.Fatalf("ensureColumns failed: %v", err)
	}

	if !db.Migrator().HasColumn(&Trader{}, "FallbackAIModelID") {
		t.Fatal("fallback_ai_model_id column was not added")
	}
	if !db.Migrator().HasColumn(&Trader{}, "StatsResetTime") {
		t.Fatal("stats_reset_time column was not added")
	}
}
