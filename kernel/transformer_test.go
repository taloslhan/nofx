package kernel

import "testing"

type appendReasoningTransformer struct{}

func (appendReasoningTransformer) Name() string { return "append_reasoning" }

func (appendReasoningTransformer) Transform(decision Decision) *Decision {
	decision.Reasoning += " | chain"
	decision.TransformApplied = append(decision.TransformApplied, "append_reasoning")
	return &decision
}

func TestTransformerPipelineNoop(t *testing.T) {
	pipeline := NewDecisionPipeline()
	decisions := []Decision{{Symbol: "BTCUSDT", Action: "hold", Reasoning: "wait"}}

	results := pipeline.Apply(decisions)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Transformed.Action != "hold" {
		t.Fatalf("expected hold action, got %s", results[0].Transformed.Action)
	}
}

func TestTransformerReverseOpenLong(t *testing.T) {
	pipeline := NewDecisionPipeline(NewReverseTransformer(ReverseConfig{
		Enabled:       true,
		SLTPMode:      ReverseSLTPModeSwap,
		LeverageScale: 1,
		PositionScale: 1,
	}))

	results := pipeline.Apply([]Decision{{
		Symbol:          "BTCUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 100,
		StopLoss:        84000,
		TakeProfit:      88000,
		Reasoning:       "trend continuation",
	}})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	got := results[0].Transformed
	if got.Action != "open_short" {
		t.Fatalf("expected open_short, got %s", got.Action)
	}
	if got.StopLoss != 88000 || got.TakeProfit != 84000 {
		t.Fatalf("expected swapped SL/TP, got stop=%v take=%v", got.StopLoss, got.TakeProfit)
	}
	if got.OriginalAction != "open_long" {
		t.Fatalf("expected original action open_long, got %s", got.OriginalAction)
	}
	if got.Reasoning != "[REVERSED] trend continuation" {
		t.Fatalf("unexpected reasoning: %s", got.Reasoning)
	}
	if len(got.TransformApplied) != 1 || got.TransformApplied[0] != "reverse" {
		t.Fatalf("unexpected applied transforms: %#v", got.TransformApplied)
	}
}

func TestTransformerReverseOpenShort(t *testing.T) {
	pipeline := NewDecisionPipeline(NewReverseTransformer(ReverseConfig{
		Enabled:       true,
		SLTPMode:      ReverseSLTPModeSwap,
		LeverageScale: 1,
		PositionScale: 1,
	}))

	results := pipeline.Apply([]Decision{{
		Symbol:          "ETHUSDT",
		Action:          "open_short",
		Leverage:        3,
		PositionSizeUSD: 200,
		StopLoss:        3100,
		TakeProfit:      2800,
		Reasoning:       "breakdown",
	}})

	got := results[0].Transformed
	if got.Action != "open_long" {
		t.Fatalf("expected open_long, got %s", got.Action)
	}
	if got.StopLoss != 2800 || got.TakeProfit != 3100 {
		t.Fatalf("expected swapped SL/TP, got stop=%v take=%v", got.StopLoss, got.TakeProfit)
	}
}

func TestTransformerReverseLeavesExitActions(t *testing.T) {
	pipeline := NewDecisionPipeline(NewReverseTransformer(ReverseConfig{Enabled: true}))
	actions := []string{"close_long", "close_short", "hold", "wait"}

	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			results := pipeline.Apply([]Decision{{Symbol: "BTCUSDT", Action: action, Reasoning: "noop"}})
			got := results[0].Transformed
			if got.Action != action {
				t.Fatalf("expected %s, got %s", action, got.Action)
			}
			if got.OriginalAction != "" {
				t.Fatalf("expected empty original action, got %s", got.OriginalAction)
			}
			if len(got.TransformApplied) != 0 {
				t.Fatalf("expected no transforms, got %#v", got.TransformApplied)
			}
		})
	}
}

func TestTransformerReverseDisabled(t *testing.T) {
	pipeline := NewDecisionPipeline(NewReverseTransformer(ReverseConfig{Enabled: false}))
	results := pipeline.Apply([]Decision{{Symbol: "BTCUSDT", Action: "open_long", Reasoning: "noop"}})

	if results[0].Transformed.Action != "open_long" {
		t.Fatalf("expected open_long, got %s", results[0].Transformed.Action)
	}
}

func TestTransformerReverseScaling(t *testing.T) {
	pipeline := NewDecisionPipeline(NewReverseTransformer(ReverseConfig{
		Enabled:            true,
		SLTPMode:           ReverseSLTPModeSwap,
		LeverageScale:      0.5,
		PositionScale:      0.25,
		MinRiskRewardRatio: 0.5,
	}))

	results := pipeline.Apply([]Decision{{
		Symbol:          "SOLUSDT",
		Action:          "open_long",
		Leverage:        10,
		PositionSizeUSD: 400,
		StopLoss:        100,
		TakeProfit:      120,
	}})

	got := results[0].Transformed
	if got.Leverage != 5 {
		t.Fatalf("expected leverage 5, got %d", got.Leverage)
	}
	if got.PositionSizeUSD != 100 {
		t.Fatalf("expected position size 100, got %v", got.PositionSizeUSD)
	}
}

func TestTransformerPipelineChaining(t *testing.T) {
	pipeline := NewDecisionPipeline(
		NewReverseTransformer(ReverseConfig{Enabled: true, SLTPMode: ReverseSLTPModeSwap}),
		appendReasoningTransformer{},
	)

	results := pipeline.Apply([]Decision{{
		Symbol:          "BTCUSDT",
		Action:          "open_long",
		StopLoss:        84000,
		TakeProfit:      88000,
		Reasoning:       "trend continuation",
		PositionSizeUSD: 100,
		Leverage:        5,
	}})

	got := results[0].Transformed
	if got.Action != "open_short" {
		t.Fatalf("expected open_short, got %s", got.Action)
	}
	if got.Reasoning != "[REVERSED] trend continuation | chain" {
		t.Fatalf("unexpected reasoning: %s", got.Reasoning)
	}
	if len(got.TransformApplied) != 2 {
		t.Fatalf("expected 2 transforms, got %#v", got.TransformApplied)
	}
	if got.TransformApplied[0] != "reverse" || got.TransformApplied[1] != "append_reasoning" {
		t.Fatalf("unexpected transform chain: %#v", got.TransformApplied)
	}
}

func TestTransformerReverseRecalculatePreservesOriginalSLTP(t *testing.T) {
	pipeline := NewDecisionPipeline(NewReverseTransformer(ReverseConfig{
		Enabled:  true,
		SLTPMode: ReverseSLTPModeRecalculate,
	}))

	results := pipeline.Apply([]Decision{{
		Symbol:     "BTCUSDT",
		Action:     "open_long",
		StopLoss:   84000,
		TakeProfit: 88000,
	}})

	got := results[0].Transformed
	if got.Action != "open_short" {
		t.Fatalf("expected open_short, got %s", got.Action)
	}
	if got.StopLoss != 84000 || got.TakeProfit != 88000 {
		t.Fatalf("expected SL/TP unchanged before execution, got stop=%v take=%v", got.StopLoss, got.TakeProfit)
	}
	if got.OriginalStopLoss != 84000 || got.OriginalTakeProfit != 88000 {
		t.Fatalf("expected original SL/TP to be preserved, got original stop=%v take=%v", got.OriginalStopLoss, got.OriginalTakeProfit)
	}
}

func TestTransformerReverseNoneLeavesSLTPUntouched(t *testing.T) {
	pipeline := NewDecisionPipeline(NewReverseTransformer(ReverseConfig{
		Enabled:  true,
		SLTPMode: ReverseSLTPModeNone,
	}))

	results := pipeline.Apply([]Decision{{
		Symbol:     "ETHUSDT",
		Action:     "open_short",
		StopLoss:   3100,
		TakeProfit: 2800,
	}})

	got := results[0].Transformed
	if got.Action != "open_long" {
		t.Fatalf("expected open_long, got %s", got.Action)
	}
	if got.StopLoss != 3100 || got.TakeProfit != 2800 {
		t.Fatalf("expected SL/TP unchanged, got stop=%v take=%v", got.StopLoss, got.TakeProfit)
	}
	if got.OriginalStopLoss != 0 || got.OriginalTakeProfit != 0 {
		t.Fatalf("expected no preserved originals, got original stop=%v take=%v", got.OriginalStopLoss, got.OriginalTakeProfit)
	}
}

func TestTransformerReverseLegacySwapCompat(t *testing.T) {
	pipeline := NewDecisionPipeline(NewReverseTransformer(ReverseConfig{
		Enabled:  true,
		SwapSLTP: true,
	}))

	results := pipeline.Apply([]Decision{{
		Symbol:     "SOLUSDT",
		Action:     "open_long",
		StopLoss:   100,
		TakeProfit: 120,
	}})

	got := results[0].Transformed
	if got.StopLoss != 120 || got.TakeProfit != 100 {
		t.Fatalf("expected legacy swap compatibility, got stop=%v take=%v", got.StopLoss, got.TakeProfit)
	}
}
