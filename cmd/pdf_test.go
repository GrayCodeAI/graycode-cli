package cmd

import (
	"bytes"
	"compress/zlib"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalPDF returns a tiny but structurally valid PDF whose content stream
// shows the given text via a Tj operator. If compress is true the stream body
// is FlateDecode (zlib) compressed.
func minimalPDF(t *testing.T, text string, compress bool) []byte {
	t.Helper()
	stream := "BT /F1 12 Tf 72 720 Td (" + text + ") Tj ET"
	body := []byte(stream)
	if compress {
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		body = buf.Bytes()
	}
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	pdf.WriteString("1 0 obj\n<< /Length ")
	pdf.WriteString(itoa(len(body)))
	pdf.WriteString(" >>\nstream\n")
	pdf.Write(body)
	pdf.WriteString("\nendstream\nendobj\n")
	pdf.WriteString("trailer\n<< >>\n%%EOF\n")
	return pdf.Bytes()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestExtractPDFText_Uncompressed(t *testing.T) {
	t.Parallel()
	pdf := minimalPDF(t, "Hello PDF World", false)
	got := extractPDFText(pdf)
	if !strings.Contains(got, "Hello PDF World") {
		t.Errorf("extractPDFText = %q, want it to contain 'Hello PDF World'", got)
	}
}

func TestExtractPDFText_FlateCompressed(t *testing.T) {
	t.Parallel()
	pdf := minimalPDF(t, "Compressed Body Text", true)
	got := extractPDFText(pdf)
	if !strings.Contains(got, "Compressed Body Text") {
		t.Errorf("extractPDFText = %q, want it to contain 'Compressed Body Text'", got)
	}
}

func TestExtractPDFText_Escapes(t *testing.T) {
	t.Parallel()
	pdf := minimalPDF(t, `a\(b\)c`, false)
	got := extractPDFText(pdf)
	if !strings.Contains(got, "a(b)c") {
		t.Errorf("extractPDFText = %q, want escaped parens decoded to 'a(b)c'", got)
	}
}

func TestReadPDFText_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(path, minimalPDF(t, "Quarterly Report", false), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDFText(path)
	if err != nil {
		t.Fatalf("ReadPDFText: %v", err)
	}
	if !strings.Contains(got, "Quarterly Report") {
		t.Errorf("ReadPDFText = %q, want it to contain 'Quarterly Report'", got)
	}
}

func TestReadPDFText_NotFound(t *testing.T) {
	t.Parallel()
	if _, err := ReadPDFText("/nonexistent/file.pdf"); err == nil {
		t.Error("expected error for missing pdf")
	}
}

func TestReadPDFText_NotPDFExtension(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPDFText(path); err == nil {
		t.Error("expected error for non-pdf extension")
	}
}

func TestReadPDFText_InvalidHeader(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.pdf")
	if err := os.WriteFile(path, []byte("not a pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPDFText(path); err == nil {
		t.Error("expected error for invalid pdf header")
	}
}

func TestIsPDFFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want bool
	}{
		{"report.pdf", true},
		{"REPORT.PDF", true},
		{"image.png", false},
		{"notes.txt", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := IsPDFFile(tc.path); got != tc.want {
			t.Errorf("IsPDFFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestExtractPDFPath(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(pdfPath, minimalPDF(t, "x", false), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := extractPDFPath("please read @" + pdfPath); got != pdfPath {
		t.Errorf("extractPDFPath @-mention = %q, want %q", got, pdfPath)
	}
	if got := extractPDFPath("look at " + pdfPath + " thanks"); got != pdfPath {
		t.Errorf("extractPDFPath bare = %q, want %q", got, pdfPath)
	}
	if got := extractPDFPath("no attachment here"); got != "" {
		t.Errorf("extractPDFPath no-match = %q, want empty", got)
	}
	if got := extractPDFPath("missing /tmp/does-not-exist.pdf"); got != "" {
		t.Errorf("extractPDFPath nonexistent = %q, want empty", got)
	}
}
