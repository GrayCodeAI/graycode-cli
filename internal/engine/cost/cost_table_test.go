package cost

import "testing"

func TestRegisterLivePricing_OverridesCatalog(t *testing.T) {
	RegisterLivePricing("mimo-v2.5-pro", 0.42, 1.7)
	in, out := ModelPricing("mimo-v2.5-pro")
	if in != 0.42 || out != 1.7 {
		t.Fatalf("ModelPricing = (%f, %f), want (0.42, 1.7)", in, out)
	}
}
