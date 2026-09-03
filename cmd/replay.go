package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/GrayCodeAI/graycode-cli/internal/terminal/tape"
)

var (
	replayJSON      bool
	replayFrames    bool
	replayGolden    string
	replayFramesDir string
)

var replayCmd = &cobra.Command{
	Use:   "replay <tape.fxtape>",
	Short: "Replay a recorded terminal capture (fxtape)",
	Long: `Replay a terminal capture recorded by the FX_RECORD tape writer (the
binary fxtape format from vercel-labs/fx, which graycode's tape package reads
byte-for-byte compatibly). Feeds the recorded stdout bytes into a virtual
terminal grid and prints the final visible snapshot.

Flags:
  --json        emit a JSON summary of the tape (header + frame list).
  --frames      print a snapshot after every stdout/resize frame.
  --golden FILE write the final snapshot to FILE instead of printing it.
  --frames-dir DIR
                export per-frame artifacts (frames/NNNN.json + NNNN.grid.txt)
                plus manifest.json into DIR (fx replay --frames-dir parity).`,
	Args: cobra.ExactArgs(1),
	RunE: runReplay,
}

func init() {
	replayCmd.Flags().BoolVar(&replayJSON, "json", false, "emit JSON summary")
	replayCmd.Flags().BoolVar(&replayFrames, "frames", false, "print a snapshot per frame")
	replayCmd.Flags().StringVar(&replayGolden, "golden", "", "write final snapshot to file")
	replayCmd.Flags().StringVar(&replayFramesDir, "frames-dir", "", "write per-frame artifacts (frames/NNNN.json + NNNN.grid.txt) and manifest.json to DIR")
	rootCmd.AddCommand(replayCmd)
}

// replayJSONSummary mirrors fx's --json output fields.
type replayJSONSummary struct {
	Cols        uint16            `json:"cols"`
	Rows        uint16            `json:"rows"`
	EpochMS     int64             `json:"epoch_ms"`
	Version     string            `json:"version"`
	Frames      []replayJSONFrame `json:"frames"`
	FrameCount  int               `json:"frame_count"`
	ResizeCount int               `json:"resize_count"`
	StdoutBytes int               `json:"stdout_bytes"`
}

type replayJSONFrame struct {
	DeltaMS int32  `json:"delta_ms"`
	Kind    string `json:"kind"`
	Len     int    `json:"len"`
}

func runReplay(cmd *cobra.Command, args []string) error {
	path := args[0]
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("replay: cannot read %s: %w", path, err)
	}
	t, err := tape.Parse(data)
	if err != nil {
		return fmt.Errorf("replay: bad tape %s: %w", path, err)
	}

	if replayFramesDir != "" {
		if _, err := tape.ExportFramesDir(replayFramesDir, t); err != nil {
			return fmt.Errorf("replay: --frames-dir %s: %w", replayFramesDir, err)
		}
	}

	if replayJSON {
		return emitReplayJSON(cmd, t)
	}

	replay, final := tape.ReplayTape(t)

	if replayGolden != "" {
		if err := os.WriteFile(replayGolden, []byte(final+"\n"), 0o644); err != nil {
			return fmt.Errorf("replay: cannot write golden %s: %w", replayGolden, err)
		}
		return nil
	}

	if replayFrames {
		// Replay frame-by-frame, printing a snapshot after each non-marker
		// frame (mirrors fx's --frames output).
		grid := tape.NewGrid(int(t.Header.Cols), int(t.Header.Rows))
		for i, f := range t.Frames {
			switch f.Kind {
			case tape.KindStdout:
				grid.Feed(f.Payload)
			case tape.KindResize:
				if len(f.Payload) >= 4 {
					grid.Resize(int(f.Payload[0])|int(f.Payload[1])<<8, int(f.Payload[2])|int(f.Payload[3])<<8)
				}
			case tape.KindMarker:
				continue
			}
			if f.Kind != tape.KindMarker {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n--- frame %d (%s, +%dms) ---\n", i+1, f.Kind, f.DeltaMS)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), grid.Snapshot())
			}
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "replay: %d frames, %d bytes stdout\n", replay.Frames, replay.Stdout)
		return nil
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), final)
	return nil
}

func emitReplayJSON(cmd *cobra.Command, t *tape.Tape) error {
	replay, _ := tape.ReplayTape(t)
	sum := replayJSONSummary{
		Cols:        t.Header.Cols,
		Rows:        t.Header.Rows,
		EpochMS:     t.Header.EpochMS,
		Version:     t.Header.Version,
		FrameCount:  len(t.Frames),
		StdoutBytes: replay.Stdout,
	}
	for _, f := range t.Frames {
		if f.Kind == tape.KindResize {
			sum.ResizeCount++
		}
		sum.Frames = append(sum.Frames, replayJSONFrame{
			DeltaMS: f.DeltaMS,
			Kind:    strings.ToLower(f.Kind.String()),
			Len:     len(f.Payload),
		})
	}
	out, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return fmt.Errorf("replay: marshal json: %w", err)
	}
	_, _ = cmd.OutOrStdout().Write(out)
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	return nil
}
