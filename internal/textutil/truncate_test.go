package textutil

import "testing"

func TestTruncate(t *testing.T) {
	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"empty max", "hello", 0, ""},
		{"negative max", "hello", -1, ""},
		{"under limit", "hi", 10, "hi"},
		{"exact limit", "hello", 5, "hello"},
		{"over limit", "hello world", 8, "hello..."},
		{"tiny max no room for ellipsis", "hello", 3, "hel"},
		{"multibyte runes not split", "héllo wörld", 8, "héllo..."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Truncate(c.s, c.max)
			if got != c.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
			}
		})
	}
}
