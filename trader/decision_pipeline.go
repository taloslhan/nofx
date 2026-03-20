package trader

import (
	"nofx/kernel"
	"nofx/store"
)

const defaultReverseMinRiskRewardRatio = 0.5

func buildDecisionPipeline(strategyConfig *store.StrategyConfig) *kernel.DecisionPipeline {
	if strategyConfig == nil || strategyConfig.ReverseStrategy == nil || !strategyConfig.ReverseStrategy.Enabled {
		return nil
	}

	pipeline := kernel.NewDecisionPipeline()
	pipeline.Add(kernel.NewReverseTransformer(toKernelReverseConfig(strategyConfig.ReverseStrategy)))
	return pipeline
}

func toKernelReverseConfig(config *store.ReverseStrategyConfig) kernel.ReverseConfig {
	if config == nil {
		return kernel.ReverseConfig{}
	}

	swapSLTP := true
	if config.SwapSLTP != nil {
		swapSLTP = *config.SwapSLTP
	}

	return kernel.ReverseConfig{
		Enabled:            config.Enabled,
		SwapSLTP:           swapSLTP,
		LeverageScale:      config.LeverageScale,
		PositionScale:      config.PositionScale,
		MinRiskRewardRatio: config.MinRiskRewardRatio,
	}
}

func hasDecisionTransform(decision *kernel.Decision, name string) bool {
	if decision == nil {
		return false
	}
	for _, applied := range decision.TransformApplied {
		if applied == name {
			return true
		}
	}
	return false
}

func (at *AutoTrader) getReverseMinRiskRewardRatio() float64 {
	if at == nil || at.config.StrategyConfig == nil || at.config.StrategyConfig.ReverseStrategy == nil {
		return defaultReverseMinRiskRewardRatio
	}

	ratio := at.config.StrategyConfig.ReverseStrategy.MinRiskRewardRatio
	if ratio <= 0 {
		return defaultReverseMinRiskRewardRatio
	}
	return ratio
}
