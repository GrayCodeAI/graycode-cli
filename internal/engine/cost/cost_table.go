package cost

import "github.com/GrayCodeAI/hawk/internal/provider/routing"

func ModelPricing(modelName string) (inputPricePerM, outputPricePerM float64) {
	info, ok := routing.Find(modelName)
	if !ok {
		return 3.0, 15.0
	}
	if info.InputPrice <= 0 && info.OutputPrice <= 0 {
		return 3.0, 15.0
	}
	return info.InputPrice, info.OutputPrice
}
