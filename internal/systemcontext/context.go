// Package systemcontext implements a composable, incrementally-rendered
// model context runtime inspired by opencode's System Context architecture.
//
// Instead of re-rendering a monolithic system prompt on every turn, privileged
// context is modelled as a set of independently refreshable typed Sources, each
// with a stable key, a JSON codec, an infallible loader, and pure renderers.
// A Reconciler compares each loaded source against a durable snapshot and
// returns exactly one action: unchanged, a single combined update message, or
// a full replacement. This preserves a provider-cache-stable baseline across
// turns while still admitting changed context as a chronological update.
package systemcontext

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Key is a stable, namespaced identity for a context source. The string form
// follows "scope/name" so contributions are deterministic when sorted.
type Key struct {
	scope string
	name  string
}

// NewKey builds a namespaced, stable key. Both parts must be non-empty.
func NewKey(scope, name string) Key {
	return Key{scope: scope, name: name}
}

// String renders the key in its stable "scope/name" form.
func (k Key) String() string {
	if k.scope == "" {
		return k.name
	}
	return k.scope + "/" + k.name
}

// Valid reports whether the key has both non-empty parts.
func (k Key) Valid() bool {
	return k.scope != "" && k.name != ""
}

// Unavailable is a sentinel returned by loaders when a source value is
// temporarily unobservable. It is distinct from a successfully loaded absence,
// which may emit removal text. Reconcile retains the prior effective value for
// an unavailable source (stale-while-revalidate) rather than dropping it.
type Unavailable struct{}

// ErrDuplicateKey is returned when combining sources with the same key.
var ErrDuplicateKey = fmt.Errorf("systemcontext: duplicate source key")

// Source is a single independently observed typed context value.
//
// The generic parameter V is the source's value type. The codec serializes V to
// a comparable JSON form for snapshotting and equality, and the renderers
// produce model-visible text only when needed.
//
// Sources form a fixed set for the lifetime of a SystemContext. A change to the
// set of sources (add/remove) is a composition change and forces a fresh
// baseline (Replace) rather than an incremental update.
type Source[V any] struct {
	Key      Key
	Codec    Codec[V]
	Load     func() (V, error)
	Baseline func(current V) string
	Update   func(previous, current V) string
}

// Codec serializes a typed value to and from a stable JSON form used for
// durable comparison snapshots.
type Codec[V any] struct {
	Encode func(V) (json.RawMessage, error)
	Decode func(json.RawMessage) (V, error)
	Equal  func(a, b V) bool
}

// JSONCodec builds a Codec for a JSON-serializable value type using a custom
// equality function.
func JSONCodec[V any](equal func(a, b V) bool) Codec[V] {
	return Codec[V]{
		Encode: func(v V) (json.RawMessage, error) { return json.Marshal(v) },
		Decode: func(b json.RawMessage) (V, error) {
			var v V
			err := json.Unmarshal(b, &v)
			return v, err
		},
		Equal: equal,
	}
}

// SystemContext is an opaque carrier of one or more typed sources. The value
// types are hidden behind the codec so heterogeneous sources compose.
type SystemContext struct {
	sources []typedSource
}

type typedSource struct {
	key   Key
	load  func() (json.RawMessage, error)
	base  func(json.RawMessage) string
	upd   func(prevJSON, curJSON json.RawMessage) string
	equal func(a, b json.RawMessage) bool
	valid func() bool
}

// New builds a SystemContext from one or more sources, rejecting duplicate keys.
func New[V any](s Source[V]) *SystemContext {
	return NewAll(s)
}

// NewAll combines multiple sources. Duplicate keys fail composition.
func NewAll[V any](sources ...Source[V]) *SystemContext {
	ctx := &SystemContext{}
	seen := map[string]bool{}
	for _, s := range sources {
		if !s.Key.Valid() {
			panic(fmt.Sprintf("systemcontext: source has invalid key %q", s.Key.String()))
		}
		if seen[s.Key.String()] {
			panic(fmt.Sprintf("systemcontext: duplicate source key %q", s.Key.String()))
		}
		seen[s.Key.String()] = true
		ctx.sources = append(ctx.sources, typedSource{
			key:   s.Key,
			load:  toLoad(s.Codec, s.Load),
			base:  renderBase(s.Codec, s.Baseline),
			upd:   renderUpdate(s.Codec, s.Update),
			equal: func(a, b json.RawMessage) bool { return equalJSON(s.Codec, a, b) },
			valid: func() bool { return true },
		})
	}
	return ctx
}

// Sources returns the ordered, stable-keyed source descriptors.
func (c *SystemContext) Sources() []SourceInfo {
	out := make([]SourceInfo, len(c.sources))
	for i, s := range c.sources {
		out[i] = SourceInfo{Key: s.key.String()}
	}
	return out
}

// SourceInfo is a lightweight, value-typed view of a source used by hosts for
// inspection and logging.
type SourceInfo struct {
	Key string
}

func toLoad[V any](codec Codec[V], load func() (V, error)) func() (json.RawMessage, error) {
	if load == nil {
		load = func() (V, error) { var z V; return z, nil }
	}
	return func() (json.RawMessage, error) {
		v, err := load()
		if err != nil {
			return nil, err
		}
		return codec.Encode(v)
	}
}

func renderBase[V any](codec Codec[V], fn func(V) string) func(json.RawMessage) string {
	if fn == nil {
		return func(json.RawMessage) string { return "" }
	}
	return func(raw json.RawMessage) string {
		v, err := codec.Decode(raw)
		if err != nil {
			return ""
		}
		return fn(v)
	}
}

func renderUpdate[V any](codec Codec[V], fn func(prev, cur V) string) func(json.RawMessage, json.RawMessage) string {
	if fn == nil {
		return func(json.RawMessage, json.RawMessage) string { return "" }
	}
	return func(prevRaw, curRaw json.RawMessage) string {
		prev, err1 := codec.Decode(prevRaw)
		cur, err2 := codec.Decode(curRaw)
		if err1 != nil || err2 != nil {
			return ""
		}
		return fn(prev, cur)
	}
}

func equalJSON[V any](codec Codec[V], a, b json.RawMessage) bool {
	av, err1 := codec.Decode(a)
	bv, err2 := codec.Decode(b)
	if err1 != nil || err2 != nil {
		return false
	}
	if codec.Equal != nil {
		return codec.Equal(av, bv)
	}
	return string(normalizeJSON(a)) == string(normalizeJSON(b))
}

// normalizeJSON canonicalizes a raw message by compacting it, so semantic JSON
// equality works regardless of whitespace.
func normalizeJSON(raw json.RawMessage) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return raw
	}
	return buf.Bytes()
}

// Snapshot is a durable, codec-encoded view of every source's last-admitted
// value. It is the model-hidden JSON state used to compare each source with
// its last admitted value.
type Snapshot struct {
	// Values maps stable key string -> encoded value.
	Values map[string]json.RawMessage `json:"values,omitempty"`
}

// NewSnapshot returns an empty snapshot.
func NewSnapshot() *Snapshot {
	return &Snapshot{Values: map[string]json.RawMessage{}}
}

// Marshal / Unmarshal are provided for hosts that persist the snapshot.
func (s *Snapshot) Marshal() ([]byte, error) { return json.Marshal(s) }
func (s *Snapshot) Unmarshal(b []byte) error { return json.Unmarshal(b, s) }

// loadedValue is the outcome of loading a single source.
type loadedValue struct {
	key   string
	raw   json.RawMessage
	avail bool
}

// observe runs every source loader, returning one entry per source. Loader
// errors are treated as Unavailable (stale-while-revalidate) rather than
// failing the whole observation.
func (c *SystemContext) observe() []loadedValue {
	out := make([]loadedValue, len(c.sources))
	for i, s := range c.sources {
		out[i] = loadedValue{key: s.key.String()}
		raw, err := s.load()
		if err != nil {
			out[i].avail = false
			continue
		}
		out[i].raw = raw
		out[i].avail = true
	}
	return out
}
