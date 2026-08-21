package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/terminal/tape"
	"github.com/spf13/cobra"
)

var (
	tapeStatusJSON bool
	tapeCommitName string
	tapeCommitDir  string
)

var tapeCmd = &cobra.Command{
	Use:   "tape",
	Short: "Inspect and checkpoint recorded terminal captures (fxtape)",
	Long: `tape inspects and checkpoints recorded terminal captures in the binary
fxtape format. "tape status" summarizes a tape's header, frame mix, and
footprint; "tape commit" copies a validated tape into a named location with a
content hash so a session can be recalled as an immutable artifact.`,
}

var tapeStatusCmd = &cobra.Command{
	Use:   "status <tape.fxtape>",
	Short: "Summarize a tape's header, frames, and footprint",
	Long: `Print tape metrics: terminal size, capture time, version, frame count,
per-kind frame breakdown, stdout bytes, and total duration.`,
	Args: cobra.ExactArgs(1),
	RunE: runTapeStatus,
}

var tapeCommitCmd = &cobra.Command{
	Use:   "commit <tape.fxtape>",
	Short: "Checkpoint a tape to a named immutable artifact",
	Long: `Copy a validated tape into the commit store under a name, forbidding
overwrites, and write a sidecar meta.json with the content hash and commit ID.`,
	Args: cobra.ExactArgs(1),
	RunE: runTapeCommit,
}

func init() {
	tapeStatusCmd.Flags().BoolVar(&tapeStatusJSON, "json", false, "output status as JSON")
	tapeCommitCmd.Flags().StringVar(&tapeCommitName, "name", "", "commit name (default: source basename)")
	tapeCommitCmd.Flags().StringVar(&tapeCommitDir, "dir", "", "commit directory (default: HAWK_TAPES_DIR or user config dir)")
	tapeCmd.AddCommand(tapeStatusCmd, tapeCommitCmd)
	rootCmd.AddCommand(tapeCmd)
}

func runTapeStatus(cmd *cobra.Command, args []string) error {
	st, err := tape.InspectFile(args[0])
	if err != nil {
		return err
	}
	if tapeStatusJSON {
		out, err := json.MarshalIndent(st, "", "  ")
		if err != nil {
			return fmt.Errorf("tape status: marshal json: %w", err)
		}
		_, _ = cmd.OutOrStdout().Write(out)
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
		return nil
	}

	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "path:      %s\n", st.Path)
	_, _ = fmt.Fprintf(w, "size:      %d bytes\n", st.Size)
	_, _ = fmt.Fprintf(w, "terminal:  %dx%d\n", st.Cols, st.Rows)
	_, _ = fmt.Fprintf(w, "captured:  %s\n", time.UnixMilli(st.EpochMS).UTC().Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "version:   %s\n", st.Version)
	_, _ = fmt.Fprintf(w, "frames:    %d\n", st.FrameCount)
	_, _ = fmt.Fprintf(w, "stdout:    %d bytes\n", st.StdoutBytes)
	_, _ = fmt.Fprintf(w, "duration:  %s\n", tapeDuration(st.DurationMS))
	for _, k := range []string{"stdout", "stdin", "resize", "sigint", "marker"} {
		if n := st.Kinds[k]; n > 0 {
			_, _ = fmt.Fprintf(w, "  %-7s %d\n", k+":", n)
		}
	}
	return nil
}

func runTapeCommit(cmd *cobra.Command, args []string) error {
	src := args[0]
	name := tapeCommitName
	if name == "" {
		base := filepath.Base(src)
		name = strings.TrimSuffix(base, filepath.Ext(base))
		if !tape.ValidCommitName(name) {
			return fmt.Errorf("cannot derive a commit name from %q; use --name", src)
		}
	}
	c, err := tape.CommitFile(src, name, tapeCommitDir)
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "committed %s\n", c.Name)
	_, _ = fmt.Fprintf(w, "  id:   %s\n", c.CommitID)
	_, _ = fmt.Fprintf(w, "  tape: %s\n", c.Path)
	_, _ = fmt.Fprintf(w, "  meta: %s\n", c.MetaPath)
	return nil
}

// tapeDuration renders a millisecond span compactly.
func tapeDuration(ms int64) string {
	switch {
	case ms < 1000:
		return fmt.Sprintf("%d ms", ms)
	case ms < 60_000:
		return fmt.Sprintf("%.1f s", float64(ms)/1000)
	default:
		return fmt.Sprintf("%.1f min", float64(ms)/60_000)
	}
}
