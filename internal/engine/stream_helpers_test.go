package engine

import "testing"

func TestDetectCodeLanguage(t *testing.T) {
	tests := []struct {
		cmd      string
		wantLang string
	}{
		{"python3 script.py", "python"},
		{"python -c 'print(1)'", "python"},
		{"node app.js", "javascript"},
		{"npm install", "javascript"},
		{"go run main.go", "go"},
		{"go test ./...", "go"},
		{"go build", ""}, // build is not code dispatch
		{"ruby app.rb", "ruby"},
		{"perl script.pl", "perl"},
		{"php index.php", "php"},
		{"cargo build", ""}, // cargo build is not code dispatch
		{"cargo run", "rust"},
		{"javac Main.java", "java"},
		{"echo hello", ""}, // not a code language
		{"ls -la", ""},     // not a code language
		{"", ""},           // empty
		{"/usr/bin/python3 script.py", "python"},
	}

	for _, tt := range tests {
		got := detectCodeLanguage(tt.cmd)
		if got != tt.wantLang {
			t.Errorf("detectCodeLanguage(%q) = %q, want %q", tt.cmd, got, tt.wantLang)
		}
	}
}
