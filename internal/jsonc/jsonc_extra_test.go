package jsonc

import (
	"errors"
	"testing"
)

func TestPrettyError_NilError(t *testing.T) {
	result := PrettyError(nil)
	if result != "" {
		t.Errorf("PrettyError(nil) = %q, want empty", result)
	}
}

func TestPrettyError_WithPrefix(t *testing.T) {
	err := errors.New("jsonc: something went wrong")
	result := PrettyError(err)
	if result != "something went wrong" {
		t.Errorf("PrettyError() = %q, want %q", result, "something went wrong")
	}
}

func TestPrettyError_WithoutPrefix(t *testing.T) {
	err := errors.New("some other error")
	result := PrettyError(err)
	if result != "some other error" {
		t.Errorf("PrettyError() = %q, want %q", result, "some other error")
	}
}

func TestPrettyError_EmptyPrefix(t *testing.T) {
	err := errors.New("jsonc: ")
	result := PrettyError(err)
	if result != "" {
		t.Errorf("PrettyError() = %q, want empty", result)
	}
}
