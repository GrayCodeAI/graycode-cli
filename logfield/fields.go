package logfield

import (
	"errors"
	"sort"
	"strings"
)

var (
	ErrInvalidKey   = errors.New("invalid key: must be [A-Za-z0-9_]+")
	ErrInvalidValue = errors.New("invalid value: must not contain spaces or '='")
)

// Fields is a validated key=value annotation set.
type Fields struct {
	m map[string]string
}

func New(capacity int) *Fields {
	return &Fields{m: make(map[string]string, capacity)}
}

func (f *Fields) Put(key, value string) error {
	if !isValidKey(key) {
		return ErrInvalidKey
	}
	if !isValidValue(value) {
		return ErrInvalidValue
	}
	f.m[key] = value
	return nil
}

func (f *Fields) MustPut(key, value string) *Fields {
	if err := f.Put(key, value); err != nil {
		panic(err)
	}
	return f
}

func (f *Fields) Get(key string) (string, bool) {
	v, ok := f.m[key]
	return v, ok
}

func (f *Fields) String() string {
	if len(f.m) == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(len(f.m) * 16)
	first := true
	for k, v := range f.m {
		if !first {
			b.WriteByte(' ')
		}
		first = false
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(v)
	}
	return b.String()
}

func (f *Fields) StringSorted() string {
	if len(f.m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(f.m))
	for k := range f.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.Grow(len(keys) * 16)
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(f.m[k])
	}
	return b.String()
}

func Parse(s string, capacity int) (*Fields, error) {
	f := New(capacity)
	n := len(s)
	i := 0
	for i < n {
		for i < n && s[i] == ' ' {
			i++
		}
		if i >= n {
			break
		}
		start := i
		for i < n && s[i] != '=' {
			i++
		}
		if i == start || i >= n {
			return nil, ErrInvalidKey
		}
		key := s[start:i]
		i++
		start = i
		for i < n && s[i] != ' ' {
			i++
		}
		if i == start {
			return nil, ErrInvalidValue
		}
		if err := f.Put(key, s[start:i]); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func isValidKey(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

func isValidValue(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '=' {
			return false
		}
	}
	return true
}
