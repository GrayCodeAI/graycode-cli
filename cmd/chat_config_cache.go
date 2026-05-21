package cmd

import hawkconfig "github.com/GrayCodeAI/hawk/internal/config"

func configuredGatewayKeys() map[string]bool {
	out := map[string]bool{}
	for _, p := range hawkconfig.ConfiguredCredentialProviders() {
		out[p] = true
	}
	return out
}
