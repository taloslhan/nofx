package trader

import (
	"nofx/kernel"
	"testing"
)

func TestApplyReverseSLTPRecalculationLong(t *testing.T) {
	decision := &kernel.Decision{
		Action:             "open_long",
		StopLoss:           95,
		TakeProfit:         120,
		OriginalStopLoss:   110,
		OriginalTakeProfit: 80,
	}

	updated := applyReverseSLTPRecalculation(decision, 100)
	if !updated {
		t.Fatalf("expected recalculation to be applied")
	}
	if decision.StopLoss != 90 {
		t.Fatalf("expected recalculated stop loss 90, got %v", decision.StopLoss)
	}
	if decision.TakeProfit != 120 {
		t.Fatalf("expected recalculated take profit 120, got %v", decision.TakeProfit)
	}
}

func TestApplyReverseSLTPRecalculationShort(t *testing.T) {
	decision := &kernel.Decision{
		Action:             "open_short",
		StopLoss:           120,
		TakeProfit:         80,
		OriginalStopLoss:   90,
		OriginalTakeProfit: 110,
	}

	updated := applyReverseSLTPRecalculation(decision, 100)
	if !updated {
		t.Fatalf("expected recalculation to be applied")
	}
	if decision.StopLoss != 110 {
		t.Fatalf("expected recalculated stop loss 110, got %v", decision.StopLoss)
	}
	if decision.TakeProfit != 90 {
		t.Fatalf("expected recalculated take profit 90, got %v", decision.TakeProfit)
	}
}

func TestApplyReverseSLTPRecalculationNoopWithoutOriginals(t *testing.T) {
	decision := &kernel.Decision{
		Action:     "open_short",
		StopLoss:   120,
		TakeProfit: 80,
	}

	updated := applyReverseSLTPRecalculation(decision, 100)
	if updated {
		t.Fatalf("expected recalculation to be skipped")
	}
	if decision.StopLoss != 120 || decision.TakeProfit != 80 {
		t.Fatalf("expected decision to stay unchanged, got stop=%v take=%v", decision.StopLoss, decision.TakeProfit)
	}
}
