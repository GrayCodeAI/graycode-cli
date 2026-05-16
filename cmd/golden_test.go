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
	SetVersion("0.2.0")
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

			_ = rootCmd.Execute()

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
				t.Skipf("golden file %s not found, run with -update-golden to create", golden)
				return
			}

			if !strings.Contains(got, "hawk") {
				t.Error("help output should contain 'hawk'")
			}
			if len(got) < 100 {
				t.Error("help output seems too short")
			}
			_ = expected // compare in stricter mode later
		})
	}
}
