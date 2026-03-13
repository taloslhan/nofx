package kernel

import (
	"testing"
)

// TestLeverageFallback tests automatic correction when leverage exceeds limit
func TestLeverageFallback(t *testing.T) {
	tests := []struct {
		name            string
		decision        Decision
		accountEquity   float64
		btcEthLeverage  int
		altcoinLeverage int
		wantLeverage    int // Expected leverage after correction
		wantError       bool
	}{
		{
			name: "Altcoin leverage exceeded - auto-correct to limit",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        20, // Exceeds limit
				PositionSizeUSD: 100,
				StopLoss:        50,
				TakeProfit:      200,
			},
			accountEquity:   100,
			btcEthLeverage:  10,
			altcoinLeverage: 5, // Limit 5x
			wantLeverage:    5, // Should be corrected to 5
			wantError:       false,
		},
		{
			name: "BTC leverage exceeded - auto-correct to limit",
			decision: Decision{
				Symbol:          "BTCUSDT",
				Action:          "open_long",
				Leverage:        20, // Exceeds limit
				PositionSizeUSD: 1000,
				StopLoss:        90000,
				TakeProfit:      110000,
			},
			accountEquity:   100,
			btcEthLeverage:  10, // Limit 10x
			altcoinLeverage: 5,
			wantLeverage:    10, // Should be corrected to 10
			wantError:       false,
		},
		{
			name: "Leverage within limit - no correction",
			decision: Decision{
				Symbol:          "ETHUSDT",
				Action:          "open_short",
				Leverage:        5, // Not exceeded
				PositionSizeUSD: 500,
				StopLoss:        4000,
				TakeProfit:      3000,
			},
			accountEquity:   100,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			wantLeverage:    5, // Stays unchanged
			wantError:       false,
		},
		{
			name: "Leverage is 0 - should error",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        0, // Invalid
				PositionSizeUSD: 100,
				StopLoss:        50,
				TakeProfit:      200,
			},
			accountEquity:   100,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			wantLeverage:    0,
			wantError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use default position value ratios for testing (10x for BTC/ETH, 1.5x for altcoins)
			err := validateDecision(&tt.decision, tt.accountEquity, tt.btcEthLeverage, tt.altcoinLeverage, 10.0, 1.5, 0, 3.0, 0.5)

			// Check error status
			if (err != nil) != tt.wantError {
				t.Errorf("validateDecision() error = %v, wantError %v", err, tt.wantError)
				return
			}

			// If shouldn't error, check if leverage was correctly corrected
			if !tt.wantError && tt.decision.Leverage != tt.wantLeverage {
				t.Errorf("Leverage not corrected: got %d, want %d", tt.decision.Leverage, tt.wantLeverage)
			}
		})
	}
}


// TestRiskRewardValidation tests risk/reward ratio validation with various market price scenarios
// Inspired by cycle #190 bug: AI analyzed at price 86.81 but execution happened at 89.68,
// where TP (89.5) was already below entry price, making R:R negative.
func TestRiskRewardValidation(t *testing.T) {
	tests := []struct {
		name               string
		decision           Decision
		accountEquity      float64
		marketPrice        float64
		minRiskRewardRatio float64
		wantError          bool
		errorContains      string
	}{
		{
			name: "Cycle #190 scenario - AI analysis price (86.81) passes validation",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        5,
				PositionSizeUSD: 270,
				StopLoss:        86,
				TakeProfit:      89.5,
			},
			accountEquity:      90,
			marketPrice:        86.81, // AI analysis time price
			minRiskRewardRatio: 2.0,
			wantError:          false, // R:R = 3.32:1, passes >= 2.0
		},
		{
			name: "Cycle #190 scenario - execution price (88.5) should FAIL",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        5,
				PositionSizeUSD: 270,
				StopLoss:        86,
				TakeProfit:      89.5,
			},
			accountEquity:      90,
			marketPrice:        88.5, // Execution time price (panel entry price)
			minRiskRewardRatio: 2.0,
			wantError:          true, // R:R = 0.40:1, fails >= 2.0
			errorContains:      "risk/reward ratio too low",
		},
		{
			name: "Cycle #190 scenario - fill price (89.68) where TP < entry should FAIL",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        5,
				PositionSizeUSD: 270,
				StopLoss:        86,
				TakeProfit:      89.5,
			},
			accountEquity:      90,
			marketPrice:        89.68, // Actual fill price, TP 89.5 < entry 89.68
			minRiskRewardRatio: 2.0,
			wantError:          true, // R:R negative (reward is negative for long)
			errorContains:      "risk/reward ratio too low",
		},
		{
			name: "Long - R:R exactly at minimum (boundary)",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        5,
				PositionSizeUSD: 100,
				StopLoss:        98,
				TakeProfit:      106, // risk=2%, reward=6%, R:R=3.0
			},
			accountEquity:      100,
			marketPrice:        100,
			minRiskRewardRatio: 3.0,
			wantError:          false, // Exactly 3.0:1, should pass
		},
		{
			name: "Long - R:R slightly below minimum (2.99 < 3.0)",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        5,
				PositionSizeUSD: 100,
				StopLoss:        98,
				TakeProfit:      105.95, // risk=2%, reward=5.95%, R:R=2.975
			},
			accountEquity:      100,
			marketPrice:        100,
			minRiskRewardRatio: 3.0,
			wantError:          true,
			errorContains:      "risk/reward ratio too low",
		},
		{
			name: "Short - R:R with market price validation",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_short",
				Leverage:        5,
				PositionSizeUSD: 100,
				StopLoss:        104,
				TakeProfit:      94, // risk=4%, reward=6%, R:R=1.5
			},
			accountEquity:      100,
			marketPrice:        100,
			minRiskRewardRatio: 2.0,
			wantError:          true, // 1.5:1 < 2.0
			errorContains:      "risk/reward ratio too low",
		},
		{
			name: "Short - TP above entry (invalid for short)",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_short",
				Leverage:        5,
				PositionSizeUSD: 100,
				StopLoss:        104,
				TakeProfit:      94,
			},
			accountEquity:      100,
			marketPrice:        93, // Market price below TP, reward negative for short
			minRiskRewardRatio: 2.0,
			wantError:          true,
			errorContains:      "risk/reward ratio too low",
		},
		{
			name: "Floating point precision - should not cause false rejection",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        5,
				PositionSizeUSD: 100,
				StopLoss:        99,
				TakeProfit:      103, // risk=1%, reward=3%, R:R=3.0 (may have fp issues)
			},
			accountEquity:      100,
			marketPrice:        100,
			minRiskRewardRatio: 3.0,
			wantError:          false, // Should pass after rounding
		},
		{
			name: "No market price - uses fallback entry estimate",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        5,
				PositionSizeUSD: 100,
				StopLoss:        86,
				TakeProfit:      89.5,
			},
			accountEquity:      100,
			marketPrice:        0, // No market price available
			minRiskRewardRatio: 2.0,
			wantError:          false, // Fallback: entry = 86 + (89.5-86)*0.2 = 86.7, R:R = 4.0
		},
		{
			name: "Min R:R ratio of 0 - skips validation",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        5,
				PositionSizeUSD: 100,
				StopLoss:        99,
				TakeProfit:      100.5,
			},
			accountEquity:      100,
			marketPrice:        100,
			minRiskRewardRatio: 0, // Disabled
			wantError:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use altcoinPosRatio=5.0 to match cycle #190 strategy (max position = equity * 5)
			err := validateDecision(&tt.decision, tt.accountEquity, 20, 10, 10.0, 5.0, tt.marketPrice, tt.minRiskRewardRatio, 0)

			if (err != nil) != tt.wantError {
				t.Errorf("validateDecision() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if tt.wantError && tt.errorContains != "" && err != nil {
				if !stringContains(err.Error(), tt.errorContains) {
					t.Errorf("error should contain %q, got: %v", tt.errorContains, err)
				}
			}
		})
	}
}

// TestValidateRiskRewardExported tests the exported ValidateRiskReward function
// that will be used for pre-execution re-validation with fresh market prices
func TestValidateRiskRewardExported(t *testing.T) {
	tests := []struct {
		name        string
		action      string
		entryPrice  float64
		stopLoss    float64
		takeProfit  float64
		minRatio    float64
		wantError   bool
	}{
		{
			name:       "Long - good R:R",
			action:     "open_long",
			entryPrice: 100,
			stopLoss:   98,
			takeProfit: 106,
			minRatio:   2.0,
			wantError:  false,
		},
		{
			name:       "Long - TP below entry (cycle #190 at fill price)",
			action:     "open_long",
			entryPrice: 89.68,
			stopLoss:   86,
			takeProfit: 89.5,
			minRatio:   2.0,
			wantError:  true,
		},
		{
			name:       "Short - good R:R",
			action:     "open_short",
			entryPrice: 100,
			stopLoss:   103,
			takeProfit: 94,
			minRatio:   2.0,
			wantError:  false,
		},
		{
			name:       "Short - TP above entry",
			action:     "open_short",
			entryPrice: 93,
			stopLoss:   96,
			takeProfit: 94,
			minRatio:   2.0,
			wantError:  true,
		},
		{
			name:       "Disabled (minRatio=0)",
			action:     "open_long",
			entryPrice: 100,
			stopLoss:   99,
			takeProfit: 100.1,
			minRatio:   0,
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRiskReward(tt.action, tt.entryPrice, tt.stopLoss, tt.takeProfit, tt.minRatio)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateRiskReward() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// contains checks if string contains substring (helper function)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
