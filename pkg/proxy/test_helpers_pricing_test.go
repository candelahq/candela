package proxy

import "github.com/candelahq/candela/pkg/costcalc"

// newCalcWithTestModels creates a Calculator with dummy pricing for test models
// that were removed from production defaults (e.g. OpenAI). This lets proxy
// integration tests exercise routing, tenant attribution, budgeting, etc.
// without being blocked by the pricing gate.
func newCalcWithTestModels() *costcalc.Calculator {
	c := costcalc.New()
	c.SetPricing(costcalc.ModelPricing{
		Provider:         "openai",
		Model:            "gpt-4o",
		InputPerMillion:  2.50,
		OutputPerMillion: 10.00,
	})
	c.SetPricing(costcalc.ModelPricing{
		Provider:         "openai",
		Model:            "gpt-4o-mini",
		InputPerMillion:  0.15,
		OutputPerMillion: 0.60,
	})
	return c
}
