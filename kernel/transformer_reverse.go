package kernel

import (
	"math"
	"strings"
)

const defaultReverseMinRiskRewardRatio = 0.5

// ReverseConfig controls reverse-strategy behavior for opening decisions.
type ReverseConfig struct {
	Enabled            bool
	SwapSLTP           bool
	LeverageScale      float64
	PositionScale      float64
	MinRiskRewardRatio float64
}

func (c ReverseConfig) normalized() ReverseConfig {
	if c.LeverageScale <= 0 {
		c.LeverageScale = 1
	}
	if c.PositionScale <= 0 {
		c.PositionScale = 1
	}
	if c.MinRiskRewardRatio <= 0 {
		c.MinRiskRewardRatio = defaultReverseMinRiskRewardRatio
	}
	return c
}

// ReverseTransformer reverses open_long/open_short while leaving exits unchanged.
type ReverseTransformer struct {
	config ReverseConfig
}

// NewReverseTransformer creates a reverse transformer with normalized defaults.
func NewReverseTransformer(config ReverseConfig) *ReverseTransformer {
	return &ReverseTransformer{config: config.normalized()}
}

// Name returns the transformer identifier used in audit logs.
func (t *ReverseTransformer) Name() string {
	return "reverse"
}

// Config returns the normalized configuration.
func (t *ReverseTransformer) Config() ReverseConfig {
	if t == nil {
		return ReverseConfig{}.normalized()
	}
	return t.config
}

// Transform reverses open decisions and preserves close/hold decisions.
func (t *ReverseTransformer) Transform(decision Decision) *Decision {
	transformed := decision
	if t == nil || !t.config.Enabled {
		return &transformed
	}

	switch decision.Action {
	case "open_long":
		transformed.Action = "open_short"
	case "open_short":
		transformed.Action = "open_long"
	default:
		return &transformed
	}

	transformed.OriginalAction = decision.Action
	transformed.TransformApplied = append(copyTransforms(decision.TransformApplied), t.Name())

	if t.config.SwapSLTP {
		transformed.StopLoss = decision.TakeProfit
		transformed.TakeProfit = decision.StopLoss
	}

	if decision.Leverage > 0 {
		scaled := int(math.Round(float64(decision.Leverage) * t.config.LeverageScale))
		if scaled < 1 {
			scaled = 1
		}
		transformed.Leverage = scaled
	}

	if decision.PositionSizeUSD > 0 {
		transformed.PositionSizeUSD = decision.PositionSizeUSD * t.config.PositionScale
	}

	if decision.Reasoning == "" {
		transformed.Reasoning = "[REVERSED]"
	} else if strings.HasPrefix(decision.Reasoning, "[REVERSED]") {
		transformed.Reasoning = decision.Reasoning
	} else {
		transformed.Reasoning = "[REVERSED] " + decision.Reasoning
	}

	return &transformed
}

func copyTransforms(applied []string) []string {
	if len(applied) == 0 {
		return nil
	}
	return append([]string(nil), applied...)
}
