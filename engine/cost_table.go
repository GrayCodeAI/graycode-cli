package engine

import "github.com/GrayCodeAI/hawk/routing"

// ModelPricing returns input/output price per million tokens for a model.
func ModelPricing(modelName string) (inputPricePerM, outputPricePerM float64) {
	info, ok := routing.Find(modelName)
	if !ok {
		return 3.0, 15.0 // conservative default
	}
	return info.InputPrice, info.OutputPrice
}
