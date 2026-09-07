package cmd

import "testing"

func TestParseConfirm(t *testing.T) {
	yes := []string{"y", "Y", "yes", "Yes", "YES", " y ", "\tyes\n"}
	for _, in := range yes {
		if !parseConfirm(in) {
			t.Errorf("parseConfirm(%q) = false, want true", in)
		}
	}
	no := []string{"", "n", "N", "no", "No", "NO", "maybe", "1", "true", "yess", " yyy"}
	for _, in := range no {
		if parseConfirm(in) {
			t.Errorf("parseConfirm(%q) = true, want false", in)
		}
	}
}
