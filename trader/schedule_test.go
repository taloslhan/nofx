package trader

import (
	"testing"
	"time"
)

func TestTimeframeToDuration(t *testing.T) {
	tests := []struct {
		timeframe string
		expected  time.Duration
		wantErr   bool
	}{
		{"1m", time.Minute, false},
		{"3m", 3 * time.Minute, false},
		{"5m", 5 * time.Minute, false},
		{"15m", 15 * time.Minute, false},
		{"30m", 30 * time.Minute, false},
		{"1h", time.Hour, false},
		{"2h", 2 * time.Hour, false},
		{"4h", 4 * time.Hour, false},
		{"6h", 6 * time.Hour, false},
		{"8h", 8 * time.Hour, false},
		{"12h", 12 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"3d", 3 * 24 * time.Hour, false},
		{"1w", 7 * 24 * time.Hour, false},
		// Error cases
		{"", 0, true},
		{"x", 0, true},
		{"abc", 0, true},
		{"1x", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.timeframe, func(t *testing.T) {
			got, err := timeframeToDuration(tt.timeframe)
			if (err != nil) != tt.wantErr {
				t.Errorf("timeframeToDuration(%q) error = %v, wantErr %v", tt.timeframe, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("timeframeToDuration(%q) = %v, want %v", tt.timeframe, got, tt.expected)
			}
		})
	}
}

func TestNextCandleCloseTime(t *testing.T) {
	tests := []struct {
		name      string
		timeframe string
		now       time.Time
		expected  time.Time
	}{
		{
			name:      "1h mid-hour",
			timeframe: "1h",
			now:       time.Date(2026, 3, 15, 14, 23, 0, 0, time.UTC),
			expected:  time.Date(2026, 3, 15, 15, 0, 0, 0, time.UTC),
		},
		{
			name:      "1h exactly on boundary",
			timeframe: "1h",
			now:       time.Date(2026, 3, 15, 15, 0, 0, 0, time.UTC),
			expected:  time.Date(2026, 3, 15, 16, 0, 0, 0, time.UTC),
		},
		{
			name:      "15m mid-period",
			timeframe: "15m",
			now:       time.Date(2026, 3, 15, 14, 23, 0, 0, time.UTC),
			expected:  time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC),
		},
		{
			name:      "15m exactly on boundary",
			timeframe: "15m",
			now:       time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC),
			expected:  time.Date(2026, 3, 15, 14, 45, 0, 0, time.UTC),
		},
		{
			name:      "15m near end of hour",
			timeframe: "15m",
			now:       time.Date(2026, 3, 15, 14, 46, 0, 0, time.UTC),
			expected:  time.Date(2026, 3, 15, 15, 0, 0, 0, time.UTC),
		},
		{
			name:      "5m mid-period",
			timeframe: "5m",
			now:       time.Date(2026, 3, 15, 14, 22, 30, 0, time.UTC),
			expected:  time.Date(2026, 3, 15, 14, 25, 0, 0, time.UTC),
		},
		{
			name:      "4h mid-period",
			timeframe: "4h",
			now:       time.Date(2026, 3, 15, 5, 30, 0, 0, time.UTC),
			expected:  time.Date(2026, 3, 15, 8, 0, 0, 0, time.UTC),
		},
		{
			name:      "4h aligned to UTC midnight",
			timeframe: "4h",
			now:       time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			expected:  time.Date(2026, 3, 15, 4, 0, 0, 0, time.UTC),
		},
		{
			name:      "4h near end of day",
			timeframe: "4h",
			now:       time.Date(2026, 3, 15, 21, 30, 0, 0, time.UTC),
			expected:  time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "1d mid-day",
			timeframe: "1d",
			now:       time.Date(2026, 3, 15, 14, 23, 0, 0, time.UTC),
			expected:  time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "1d exactly at midnight",
			timeframe: "1d",
			now:       time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			expected:  time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "3d uses fixed UTC anchor",
			timeframe: "3d",
			now:       time.Date(2026, 3, 15, 14, 23, 0, 0, time.UTC),
			expected:  time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "1w aligns to monday UTC",
			timeframe: "1w",
			now:       time.Date(2026, 3, 18, 14, 23, 0, 0, time.UTC), // Wednesday
			expected:  time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "1w exactly on monday boundary returns next week",
			timeframe: "1w",
			now:       time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC), // Monday
			expected:  time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "3m mid-period",
			timeframe: "3m",
			now:       time.Date(2026, 3, 15, 14, 7, 30, 0, time.UTC),
			expected:  time.Date(2026, 3, 15, 14, 9, 0, 0, time.UTC),
		},
		{
			name:      "30m mid-period",
			timeframe: "30m",
			now:       time.Date(2026, 3, 15, 14, 12, 0, 0, time.UTC),
			expected:  time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC),
		},
		{
			name:      "1m second into minute",
			timeframe: "1m",
			now:       time.Date(2026, 3, 15, 14, 23, 45, 0, time.UTC),
			expected:  time.Date(2026, 3, 15, 14, 24, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nextCandleCloseTime(tt.timeframe, tt.now)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tt.expected) {
				t.Errorf("nextCandleCloseTime(%q, %v) = %v, want %v",
					tt.timeframe, tt.now.Format("15:04:05"), got.Format("2006-01-02 15:04:05"), tt.expected.Format("2006-01-02 15:04:05"))
			}
		})
	}
}

func TestNextCandleCloseTime_AlwaysFuture(t *testing.T) {
	timeframes := []string{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "8h", "12h", "1d", "3d", "1w"}
	now := time.Now().UTC()

	for _, tf := range timeframes {
		t.Run(tf, func(t *testing.T) {
			got, err := nextCandleCloseTime(tf, now)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.After(now) {
				t.Errorf("nextCandleCloseTime(%q, now) = %v, expected to be after %v", tf, got, now)
			}
		})
	}
}

func TestDurationUntilNextCandle(t *testing.T) {
	now := time.Date(2026, 3, 15, 14, 50, 0, 0, time.UTC)
	waitDuration, evalTime, err := durationUntilNextCandle("1h", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedClose := time.Date(2026, 3, 15, 15, 0, 0, 0, time.UTC)
	expectedEval := expectedClose.Add(CandleCloseDelay)
	expectedWait := expectedClose.Sub(now) + CandleCloseDelay

	if waitDuration != expectedWait {
		t.Errorf("waitDuration = %v, want %v", waitDuration, expectedWait)
	}
	if !evalTime.Equal(expectedEval) {
		t.Errorf("evalTime = %v, want %v", evalTime, expectedEval)
	}
}

func TestNextCandleCloseTime_Error(t *testing.T) {
	_, err := nextCandleCloseTime("invalid", time.Now())
	if err == nil {
		t.Error("expected error for invalid timeframe, got nil")
	}
}
