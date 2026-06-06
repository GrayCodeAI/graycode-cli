package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GrayCodeAI/tok"
	"github.com/spf13/cobra"
)

// hawk embeds the tok library directly (see internal/engine/token/tokenizer.go).
// These verbs expose tok's compression, token-estimation, and secret-scanning
// surface through the hawk CLI — tok ships no standalone binary.

var (
	tokInput  string
	tokFormat string

	// compress flags
	tokIntensity string
	tokBudget    int
	tokPrompt    bool
	tokStats     bool

	// estimate flags
	tokModel string

	// scan flags
	tokRedact bool
)

var tokCmd = &cobra.Command{
	Use:   "tok",
	Short: "Token compression, estimation, and secret scanning (embedded tok library)",
	Long: `tok exposes hawk's embedded token-efficiency library:

  hawk tok compress   shrink prose/prompts or fit text to a token budget
  hawk tok estimate   count tokens and estimate cost for a model
  hawk tok scan       detect (and optionally redact) secrets in text

Input is read from --input <file>, a trailing argument, or stdin.
tok has no standalone binary — these verbs run the library in-process.`,
}

// readTokInput resolves input from --input <file>, the first positional arg,
// or stdin (in that order).
func readTokInput(args []string) (string, error) {
	if tokInput != "" {
		b, err := os.ReadFile(tokInput)
		if err != nil {
			return "", fmt.Errorf("read --input %q: %w", tokInput, err)
		}
		return string(b), nil
	}
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	if len(b) == 0 {
		return "", fmt.Errorf("no input: provide text as an argument, --input <file>, or via stdin")
	}
	return string(b), nil
}

var tokCompressCmd = &cobra.Command{
	Use:   "compress [text]",
	Short: "Compress prompts/prose or fit text to a token budget",
	Long: `Compress text with the tok pipeline.

By default uses prompt/prose compression at the chosen --intensity (lite, full,
ultra). Pass --budget to instead run the full output pipeline targeting a token
budget. Use --stats for a savings summary.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		text, err := readTokInput(args)
		if err != nil {
			return err
		}

		// --budget runs the token-budget output pipeline, unless --prompt forces
		// prose/prompt compression.
		if tokBudget > 0 && !tokPrompt {
			out, stats := tok.Compress(text, tok.WithBudget(tokBudget))
			return emitCompress(cmd, out, &stats, nil)
		}

		intensity, err := parseIntensity(tokIntensity)
		if err != nil {
			return err
		}
		out, pstats := tok.PromptCompress(text, intensity)
		return emitCompress(cmd, out, nil, &pstats)
	},
}

func parseIntensity(s string) (tok.Intensity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "full":
		return tok.IntensityFull, nil
	case "lite":
		return tok.IntensityLite, nil
	case "ultra":
		return tok.IntensityUltra, nil
	default:
		return tok.IntensityFull, fmt.Errorf("invalid --intensity %q: use lite, full, or ultra", s)
	}
}

// emitCompress prints compressed output, optionally with stats. Exactly one of
// stats / pstats is non-nil depending on which compression path ran.
func emitCompress(cmd *cobra.Command, out string, stats *tok.Stats, pstats *tok.PromptStats) error {
	if tokFormat == "json" {
		payload := map[string]any{"compressed": out}
		switch {
		case stats != nil:
			payload["stats"] = stats
		case pstats != nil:
			payload["stats"] = pstats
		}
		return writeJSON(cmd, payload)
	}

	cmd.Println(out)
	if tokStats {
		switch {
		case stats != nil:
			cmd.Println()
			cmd.Print(tok.FormatStats(*stats))
		case pstats != nil:
			cmd.Println()
			cmd.Printf("intensity=%v  bytes %d → %d (%.1f%% off)\n",
				pstats.Intensity, pstats.OriginalBytes, pstats.CompressedBytes, pstats.PercentOff)
		}
	}
	return nil
}

var tokEstimateCmd = &cobra.Command{
	Use:   "estimate [text]",
	Short: "Estimate token count and cost for a model",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		text, err := readTokInput(args)
		if err != nil {
			return err
		}

		tokens := tok.EstimateTokensForModel(text, tokModel)

		var costUSD float64
		var priced bool
		if pricing, ok := tok.GetModelPricing(tokModel); ok {
			priced = true
			costUSD = float64(tokens) / 1000.0 * pricing.InputPricePer1K
		}

		if tokFormat == "json" {
			payload := map[string]any{"tokens": tokens, "model": tokModel}
			if priced {
				payload["input_cost_usd"] = costUSD
			}
			return writeJSON(cmd, payload)
		}

		cmd.Printf("%d tokens (model: %s)\n", tokens, tokModel)
		if priced {
			cmd.Printf("≈ $%.6f input cost\n", costUSD)
		} else {
			cmd.Printf("(no pricing registered for %q — token count only)\n", tokModel)
		}
		return nil
	},
}

var tokScanCmd = &cobra.Command{
	Use:   "scan [text]",
	Short: "Detect (and optionally redact) secrets in text",
	Long:  "Scan input for credentials, keys, and other secrets. Use --redact to print the input with secrets masked.",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		text, err := readTokInput(args)
		if err != nil {
			return err
		}

		detector := tok.NewSecretDetector()

		if tokRedact {
			redacted := detector.RedactSecrets(text)
			if tokFormat == "json" {
				return writeJSON(cmd, map[string]any{"redacted": redacted})
			}
			cmd.Print(redacted)
			if !strings.HasSuffix(redacted, "\n") {
				cmd.Println()
			}
			return nil
		}

		findings := detector.DetectSecrets(text)

		if tokFormat == "json" {
			return writeJSON(cmd, map[string]any{
				"count":   len(findings),
				"secrets": findings,
			})
		}

		if len(findings) == 0 {
			cmd.Println("no secrets detected")
			return nil
		}
		cmd.Printf("%d secret(s) detected:\n", len(findings))
		for _, f := range findings {
			cmd.Printf("  - %s: %s\n", f.Type, f.Masked)
		}
		// Non-zero exit so callers (CI, hooks) can gate on detection.
		return fmt.Errorf("tok scan: %d secret(s) detected", len(findings))
	},
}

// writeJSON encodes v as indented JSON to stdout.
func writeJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func init() {
	// Shared input/output flags on each subcommand.
	for _, c := range []*cobra.Command{tokCompressCmd, tokEstimateCmd, tokScanCmd} {
		c.Flags().StringVar(&tokInput, "input", "", "read input from this file instead of stdin/args")
		c.Flags().StringVar(&tokFormat, "format", "text", "output format: text, json")
	}

	tokCompressCmd.Flags().StringVar(&tokIntensity, "intensity", "full", "prompt compression intensity: lite, full, ultra")
	tokCompressCmd.Flags().IntVar(&tokBudget, "budget", 0, "compress to fit within this many tokens (uses the output pipeline)")
	tokCompressCmd.Flags().BoolVar(&tokPrompt, "prompt", false, "force prompt/prose compression even when --budget is set")
	tokCompressCmd.Flags().BoolVar(&tokStats, "stats", false, "print a compression savings summary")

	tokEstimateCmd.Flags().StringVar(&tokModel, "model", "gpt-4o", "model to estimate tokens/cost for")

	tokScanCmd.Flags().BoolVar(&tokRedact, "redact", false, "print input with secrets masked instead of listing them")

	tokCmd.AddCommand(tokCompressCmd, tokEstimateCmd, tokScanCmd)
	rootCmd.AddCommand(tokCmd)
}
