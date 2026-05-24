package memory

import "testing"

func TestShouldAutoRemember_StrongTrigger(t *testing.T) {
	if !ShouldAutoRemember("Decision: use SQLite for local storage") {
		t.Fatal("expected decision prefix to trigger")
	}
	if !ShouldAutoRemember("Note to self: run tests with -race") {
		t.Fatal("expected note to self")
	}
}

func TestShouldAutoRemember_TwoWeakTriggers(t *testing.T) {
	if !ShouldAutoRemember("Actually, don't use globals — use dependency injection instead") {
		t.Fatal("expected two weak triggers")
	}
}

func TestShouldAutoRemember_Noise(t *testing.T) {
	if ShouldAutoRemember("Actually, here is the file you asked for.") {
		t.Fatal("single weak trigger should not remember")
	}
	if ShouldAutoRemember("") {
		t.Fatal("empty content")
	}
}
