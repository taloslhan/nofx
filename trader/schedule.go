package trader

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CandleCloseDelay is the delay after candle close before triggering evaluation.
// This ensures the exchange has finalized the OHLCV data for the closed candle.
const CandleCloseDelay = 5 * time.Second

// timeframeToDuration converts a timeframe string (e.g., "1m", "15m", "1h", "1d")
// to a time.Duration. Returns an error if the format is unrecognized.
func timeframeToDuration(timeframe string) (time.Duration, error) {
	if len(timeframe) < 2 {
		return 0, fmt.Errorf("invalid timeframe format: %q", timeframe)
	}

	unit := timeframe[len(timeframe)-1]
	valueStr := timeframe[:len(timeframe)-1]
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, fmt.Errorf("invalid timeframe value: %q", timeframe)
	}

	switch unit {
	case 'm':
		return time.Duration(value) * time.Minute, nil
	case 'h':
		return time.Duration(value) * time.Hour, nil
	case 'd':
		return time.Duration(value) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(value) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown timeframe unit %q in %q", string(unit), timeframe)
	}
}

// nextCandleCloseTime calculates the next candle close time for the given timeframe,
// relative to the provided current time. The result is always in the future.
//
// For example:
//   - timeframe="1h", now=14:23 → returns 15:00
//   - timeframe="15m", now=14:23 → returns 14:30
//   - timeframe="4h", now=14:23 → returns 16:00 (aligned to 00:00 UTC)
//   - timeframe="1h", now=15:00:00 exactly → returns 16:00 (next period, not current)
func nextCandleCloseTime(timeframe string, now time.Time) (time.Time, error) {
	duration, err := timeframeToDuration(timeframe)
	if err != nil {
		return time.Time{}, err
	}

	// Use UTC for alignment calculations to match exchange conventions
	nowUTC := now.UTC()

	// Multi-day timeframes should be aligned to a fixed UTC anchor instead of
	// "today at midnight", otherwise 3d/1w bars drift based on process start time.
	if strings.HasSuffix(timeframe, "w") {
		weekAnchor := time.Date(1970, 1, 5, 0, 0, 0, 0, time.UTC) // Monday 00:00 UTC
		return nextAnchoredCloseTime(weekAnchor, duration, nowUTC), nil
	}
	if strings.HasSuffix(timeframe, "d") {
		dayAnchor := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		return nextAnchoredCloseTime(dayAnchor, duration, nowUTC), nil
	}

	// For intraday timeframes, align to the start of the UTC day
	// then step forward in increments of the duration
	durationSecs := int64(duration.Seconds())
	dayStartUTC := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)
	elapsedSecs := int64(nowUTC.Sub(dayStartUTC).Seconds())

	// How many complete periods have elapsed since midnight
	completePeriods := elapsedSecs / durationSecs

	// Next candle close = (completePeriods + 1) * duration from midnight
	nextClose := dayStartUTC.Add(time.Duration((completePeriods + 1) * durationSecs) * time.Second)

	// If now is exactly on a candle boundary, we already incremented by 1 period
	// which gives us the next close (not the current one) — correct behavior

	return nextClose, nil
}

func nextAnchoredCloseTime(anchor time.Time, duration time.Duration, now time.Time) time.Time {
	if !now.After(anchor) {
		return anchor.Add(duration)
	}

	elapsed := now.Sub(anchor)
	periods := elapsed / duration
	next := anchor.Add((periods + 1) * duration)
	if !next.After(now) {
		next = next.Add(duration)
	}
	return next
}

// durationUntilNextCandle returns the duration to wait until the next candle close
// plus the configured delay. This is used by the scheduler to set up timers.
func durationUntilNextCandle(timeframe string, now time.Time) (time.Duration, time.Time, error) {
	nextClose, err := nextCandleCloseTime(timeframe, now)
	if err != nil {
		return 0, time.Time{}, err
	}

	waitDuration := nextClose.Sub(now) + CandleCloseDelay
	evalTime := nextClose.Add(CandleCloseDelay)

	return waitDuration, evalTime, nil
}
