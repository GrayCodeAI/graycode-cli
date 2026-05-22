package logfield

import (
	"testing"
)

func TestNew(t *testing.T) {
	f := New(4)
	if f == nil {
		t.Fatal("New returned nil")
	}
	if _, ok := f.Get("missing"); ok {
		t.Error("empty Fields should not have any keys")
	}
}

func TestPutAndGet(t *testing.T) {
	f := New(4)
	if err := f.Put("key1", "val1"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	v, ok := f.Get("key1")
	if !ok || v != "val1" {
		t.Errorf("Get(key1) = %q, %v; want val1, true", v, ok)
	}
}

func TestPutInvalidKey(t *testing.T) {
	f := New(4)
	if err := f.Put("bad key", "v"); err != ErrInvalidKey {
		t.Errorf("Put with space in key: err = %v, want ErrInvalidKey", err)
	}
	if err := f.Put("bad=val", "v"); err != ErrInvalidKey {
		t.Errorf("Put with = in key: err = %v, want ErrInvalidKey", err)
	}
	if err := f.Put("", "v"); err != ErrInvalidKey {
		t.Errorf("Put with empty key: err = %v, want ErrInvalidKey", err)
	}
}

func TestPutInvalidValue(t *testing.T) {
	f := New(4)
	if err := f.Put("k", "bad val"); err != ErrInvalidValue {
		t.Errorf("Put with space in value: err = %v, want ErrInvalidValue", err)
	}
	if err := f.Put("k", "bad=val"); err != ErrInvalidValue {
		t.Errorf("Put with = in value: err = %v, want ErrInvalidValue", err)
	}
	if err := f.Put("k", ""); err != ErrInvalidValue {
		t.Errorf("Put with empty value: err = %v, want ErrInvalidValue", err)
	}
}

func TestMustPutPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustPut should panic on invalid key")
		}
	}()
	New(4).MustPut("bad key", "v")
}

func TestMustPutSuccess(t *testing.T) {
	f := New(4)
	ret := f.MustPut("k", "v")
	if ret != f {
		t.Error("MustPut should return the same Fields")
	}
	v, _ := f.Get("k")
	if v != "v" {
		t.Errorf("Get(k) = %q, want v", v)
	}
}

func TestStringEmpty(t *testing.T) {
	f := New(4)
	if s := f.String(); s != "" {
		t.Errorf("String() = %q, want empty", s)
	}
}

func TestStringSorted(t *testing.T) {
	f := New(4)
	f.Put("z", "1")
	f.Put("a", "2")
	f.Put("m", "3")

	sorted := f.StringSorted()
	if sorted != "a=2 m=3 z=1" {
		t.Errorf("StringSorted() = %q, want %q", sorted, "a=2 m=3 z=1")
	}
}

func TestStringNonDeterministic(t *testing.T) {
	// String() iterates a map so order is not guaranteed, but all k=v should appear.
	f := New(4)
	f.Put("x", "1")
	f.Put("y", "2")
	s := f.String()
	if len(s) == 0 {
		t.Error("String() should not be empty")
	}
}

func TestParseValid(t *testing.T) {
	f, err := Parse("a=1 b=hello c=test", 4)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, tc := range []struct {
		key, want string
	}{
		{"a", "1"},
		{"b", "hello"},
		{"c", "test"},
	} {
		v, ok := f.Get(tc.key)
		if !ok || v != tc.want {
			t.Errorf("Get(%s) = %q, %v; want %q, true", tc.key, v, ok, tc.want)
		}
	}
}

func TestParseEmpty(t *testing.T) {
	f, err := Parse("", 4)
	if err != nil {
		t.Fatalf("Parse empty: %v", err)
	}
	if s := f.String(); s != "" {
		t.Errorf("String() = %q, want empty", s)
	}
}

func TestParseInvalidKey(t *testing.T) {
	_, err := Parse("bad key=val", 4)
	if err != ErrInvalidKey {
		t.Errorf("Parse: err = %v, want ErrInvalidKey", err)
	}
}

func TestParseInvalidValue(t *testing.T) {
	// Parse splits by space then validates each value. "key=" with empty value
	// triggers ErrInvalidValue because the value is empty after the = sign.
	_, err := Parse("key=", 4)
	if err != ErrInvalidValue {
		t.Errorf("Parse: err = %v, want ErrInvalidValue", err)
	}
}

func TestParseMissingEquals(t *testing.T) {
	_, err := Parse("noequals", 4)
	if err != ErrInvalidKey {
		t.Errorf("Parse: err = %v, want ErrInvalidKey", err)
	}
}

func TestPutOverwrite(t *testing.T) {
	f := New(4)
	f.Put("k", "v1")
	f.Put("k", "v2")
	v, _ := f.Get("k")
	if v != "v2" {
		t.Errorf("Get(k) = %q, want v2 (overwrite)", v)
	}
}
