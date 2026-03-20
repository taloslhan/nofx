package kernel

// DecisionTransformer mutates or filters a decision after AI generation and before execution.
// Returning nil means the decision should be dropped from the pipeline output.
type DecisionTransformer interface {
	Name() string
	Transform(decision Decision) *Decision
}

// TransformedDecision keeps both the original and final decision for audit purposes.
type TransformedDecision struct {
	Original    Decision
	Transformed Decision
	Applied     []string
}

// DecisionPipeline applies a list of transformers in order.
type DecisionPipeline struct {
	transformers []DecisionTransformer
}

// NewDecisionPipeline creates a new decision pipeline.
func NewDecisionPipeline(transformers ...DecisionTransformer) *DecisionPipeline {
	pipeline := &DecisionPipeline{}
	for _, transformer := range transformers {
		pipeline.Add(transformer)
	}
	return pipeline
}

// Add appends a transformer to the pipeline.
func (p *DecisionPipeline) Add(transformer DecisionTransformer) *DecisionPipeline {
	if p == nil || transformer == nil {
		return p
	}
	p.transformers = append(p.transformers, transformer)
	return p
}

// Apply runs all transformers against the provided decisions in sequence.
func (p *DecisionPipeline) Apply(decisions []Decision) []TransformedDecision {
	if len(decisions) == 0 {
		return nil
	}

	results := make([]TransformedDecision, 0, len(decisions))
	for _, decision := range decisions {
		current := decision
		filtered := false

		for _, transformer := range p.transformers {
			if transformer == nil {
				continue
			}

			next := transformer.Transform(current)
			if next == nil {
				filtered = true
				break
			}
			current = *next
		}

		if filtered {
			continue
		}

		applied := append([]string(nil), current.TransformApplied...)
		results = append(results, TransformedDecision{
			Original:    decision,
			Transformed: current,
			Applied:     applied,
		})
	}

	return results
}
