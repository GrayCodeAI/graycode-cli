package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/snapshot"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func (m chatModel) handleSnapshot(text string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		return m.showSnapshotList()
	}

	sub := parts[1]
	switch sub {
	case "list", "ls":
		return m.showSnapshotList()
	case "restore":
		if len(parts) < 3 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /snapshot restore <hash>"})
			return m, nil
		}
		return m.restoreSnapshot(parts[2])
	case "diff":
		if len(parts) < 3 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /snapshot diff <hash>"})
			return m, nil
		}
		return m.diffSnapshot(parts[2])
	default:
		m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /snapshot [list|restore <hash>|diff <hash>]"})
		return m, nil
	}
}

func (m chatModel) showSnapshotList() (tea.Model, tea.Cmd) {
	cwd, _ := os.Getwd()
	t := snapshot.New(cwd)
	if err := t.Init(); err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "snapshot init: " + err.Error()})
		return m, nil
	}

	history, err := t.History(15)
	if err != nil || len(history) == 0 {
		m.messages = append(m.messages, displayMsg{role: "system", content: "No snapshots yet. Snapshots are created automatically when hawk modifies files."})
		return m, nil
	}

	var sb strings.Builder
	sb.WriteString("Recent snapshots:\n\n")
	for _, p := range history {
		ts := p.Timestamp.Format("15:04:05")
		sb.WriteString(fmt.Sprintf("  %s  %s  %s\n", p.Hash, ts, p.Message))
	}
	sb.WriteString("\nUse /snapshot restore <hash> to roll back.")
	m.messages = append(m.messages, displayMsg{role: "system", content: sb.String()})
	return m, nil
}

func (m chatModel) restoreSnapshot(hash string) (tea.Model, tea.Cmd) {
	cwd, _ := os.Getwd()
	t := snapshot.New(cwd)
	if err := t.Init(); err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "snapshot init: " + err.Error()})
		return m, nil
	}

	if err := t.Restore(hash); err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "restore failed: " + err.Error()})
		return m, nil
	}

	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Restored snapshot %q — file changes reverted.", hash)})
	return m, nil
}

func (m chatModel) diffSnapshot(hash string) (tea.Model, tea.Cmd) {
	cwd, _ := os.Getwd()
	t := snapshot.New(cwd)
	if err := t.Init(); err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "snapshot init: " + err.Error()})
		return m, nil
	}

	// Get current HEAD
	history, _ := t.History(1)
	if len(history) == 0 {
		m.messages = append(m.messages, displayMsg{role: "error", content: "no snapshots to diff against"})
		return m, nil
	}

	diffs, err := t.Diff(hash, history[0].Hash)
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "diff failed: " + err.Error()})
		return m, nil
	}

	if len(diffs) == 0 {
		m.messages = append(m.messages, displayMsg{role: "system", content: "No changes between " + hash + " and current."})
		return m, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Changes since %s:\n\n", hash))
	for _, d := range diffs {
		sb.WriteString(fmt.Sprintf("  %s  +%d -%d  %s\n", d.Status[:1], d.Additions, d.Deletions, d.File))
	}
	m.messages = append(m.messages, displayMsg{role: "system", content: sb.String()})
	return m, nil
}

// CLI subcommand

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage file snapshots (undo any change)",
	Long:  "View, restore, and diff file snapshots. Hawk automatically snapshots every file modification.",
}

var snapshotListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent snapshots",
	RunE: func(_ *cobra.Command, _ []string) error {
		cwd, _ := os.Getwd()
		t := snapshot.New(cwd)
		if err := t.Init(); err != nil {
			return err
		}
		history, err := t.History(20)
		if err != nil {
			return err
		}
		if len(history) == 0 {
			fmt.Println("No snapshots yet.")
			return nil
		}
		for _, p := range history {
			fmt.Printf("%s  %s  %s\n", p.Hash, p.Timestamp.Format("2006-01-02 15:04:05"), p.Message)
		}
		return nil
	},
}

var snapshotRestoreCmd = &cobra.Command{
	Use:   "restore <hash>",
	Short: "Restore files to a snapshot",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		t := snapshot.New(cwd)
		if err := t.Init(); err != nil {
			return err
		}
		if err := t.Restore(args[0]); err != nil {
			return err
		}
		fmt.Printf("Restored to snapshot %s\n", args[0])
		return nil
	},
}

var snapshotDiffCmd = &cobra.Command{
	Use:   "diff <hash>",
	Short: "Show changes since a snapshot",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		t := snapshot.New(cwd)
		if err := t.Init(); err != nil {
			return err
		}
		history, _ := t.History(1)
		if len(history) == 0 {
			fmt.Println("No snapshots to diff against.")
			return nil
		}
		diffs, err := t.Diff(args[0], history[0].Hash)
		if err != nil {
			return err
		}
		if len(diffs) == 0 {
			fmt.Println("No changes.")
			return nil
		}
		for _, d := range diffs {
			fmt.Printf("%s  +%d -%d  %s\n", d.Status, d.Additions, d.Deletions, d.File)
		}
		return nil
	},
}

func init() {
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotRestoreCmd)
	snapshotCmd.AddCommand(snapshotDiffCmd)
}
