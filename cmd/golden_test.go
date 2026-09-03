package cmd

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update-golden", false, "update golden files")

func TestGoldenHelp(t *testing.T) {
	SetVersion("0.1.0")
	SetBuildDate("test")

	tests := []struct {
		name string
		args []string
		file string
	}{
		{"root help", []string{"--help"}, "help_root.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			rootCmd.SetOut(buf)
			rootCmd.SetErr(buf)
			rootCmd.SetArgs(tt.args)

			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("root command execute: %v", err)
			}

			got := buf.String()
			golden := filepath.Join("..", "testdata", "golden", tt.file)

			if *updateGolden {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			expected, err := os.ReadFile(golden)
			if err != nil {
				// A missing golden is a real failure: new commands should
				// force a deliberate golden update, not a silent skip.
				t.Fatalf("golden file %s not found (run with -update-golden to create): %v", golden, err)
			}

			if !strings.Contains(got, "graycode") {
				t.Error("help output should contain 'graycode'")
			}
			if len(got) < 100 {
				t.Error("help output seems too short")
			}
			if got != string(expected) {
				t.Errorf("help output does not match golden file %s\n--- got ---\n%s\n--- want (golden) ---\n%s", golden, got, expected)
			}
		})
	}
}
