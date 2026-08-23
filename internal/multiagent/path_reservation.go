// path_reservation.go implements mission-level file-path reservation,
// adopting Luvus' "reserve file paths / coordinate dependent tasks"
// orchestration primitive: agents claim the paths they touch so the
// orchestrator can detect overlapping changes between parallel branches
// before merge and sequence dependent tasks instead of discovering
// conflicts in git.
package mission

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Reservation is one agent's claim over a set of paths.
type Reservation struct {
	AgentID   string    `json:"agent_id"`
	FeatureID string    `json:"feature_id,omitempty"`
	Paths     []string  `json:"paths"`
	ClaimedAt time.Time `json:"claimed_at"`
}

// Conflict reports paths claimed by more than one agent.
type Conflict struct {
	Path   string   `json:"path"`
	Agents []string `json:"agents"`
}

// PathReservationLedger tracks which agent holds which paths. Thread-safe.
type PathReservationLedger struct {
	mu      sync.Mutex
	byPath  map[string]*Reservation // canonical path -> holder
	byAgent map[string][]string     // agent id -> held canonical paths
}

// NewPathReservationLedger creates an empty ledger.
func NewPathReservationLedger() *PathReservationLedger {
	return &PathReservationLedger{
		byPath:  map[string]*Reservation{},
		byAgent: map[string][]string{},
	}
}

func canonPath(p string) string {
	return strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(p)), "./")
}

// Reserve claims paths for agentID. If any path is already held by a
// DIFFERENT agent, nothing is claimed and the conflicting holders are
// returned. Re-reserving paths the same agent already holds is a no-op
// for those paths (idempotent).
func (l *PathReservationLedger) Reserve(agentID, featureID string, paths []string) error {
	if agentID == "" {
		return fmt.Errorf("multiagent: reserve requires an agent id")
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check conflicts first; claim nothing on any conflict (all-or-nothing).
	var conflicts []string
	for _, p := range paths {
		cp := canonPath(p)
		if cp == "" {
			continue
		}
		if holder, ok := l.byPath[cp]; ok && holder.AgentID != agentID {
			conflicts = append(conflicts, fmt.Sprintf("%s (held by %s)", cp, holder.AgentID))
		}
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("multiagent: path reservation conflict for %s: %s", agentID, strings.Join(conflicts, ", "))
	}

	now := time.Now()
	for _, p := range paths {
		cp := canonPath(p)
		if cp == "" {
			continue
		}
		if _, ok := l.byPath[cp]; !ok {
			l.byPath[cp] = &Reservation{AgentID: agentID, FeatureID: featureID, Paths: []string{cp}, ClaimedAt: now}
			l.byAgent[agentID] = append(l.byAgent[agentID], cp)
		}
	}
	return nil
}

// Release drops every path held by agentID. Returns the number released.
func (l *PathReservationLedger) Release(agentID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	held := l.byAgent[agentID]
	for _, cp := range held {
		if r, ok := l.byPath[cp]; ok && r.AgentID == agentID {
			delete(l.byPath, cp)
		}
	}
	delete(l.byAgent, agentID)
	return len(held)
}

// Holder returns the agent currently holding path ("" when free).
func (l *PathReservationLedger) Holder(path string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r, ok := l.byPath[canonPath(path)]; ok {
		return r.AgentID
	}
	return ""
}

// HeldBy lists all paths currently held by agentID, sorted.
func (l *PathReservationLedger) HeldBy(agentID string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := append([]string{}, l.byAgent[agentID]...)
	sort.Strings(out)
	return out
}

// Overlap is a pair of features whose changed-file sets intersect.
type Overlap struct {
	FeatureA string   `json:"feature_a"`
	FeatureB string   `json:"feature_b"`
	Paths    []string `json:"paths"`
}

// DetectFileOverlaps compares completed handoffs pairwise and reports every
// pair of features that touched the same files — the merge-conflict forecast.
// Deterministic ordering (by feature id then path).
func DetectFileOverlaps(handoffs map[string][]string) []Overlap {
	ids := make([]string, 0, len(handoffs))
	for id := range handoffs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	pathOwners := map[string][]string{} // path -> sorted feature ids
	for _, id := range ids {
		seen := map[string]bool{}
		for _, p := range handoffs[id] {
			cp := canonPath(p)
			if cp == "" || seen[cp] {
				continue
			}
			seen[cp] = true
			pathOwners[cp] = append(pathOwners[cp], id)
		}
	}

	pairKey := map[string]*Overlap{}
	var keys []string
	for _, p := range sortedPaths(pathOwners) {
		owners := pathOwners[p]
		if len(owners) < 2 {
			continue
		}
		for i := 0; i < len(owners); i++ {
			for j := i + 1; j < len(owners); j++ {
				k := owners[i] + "\x00" + owners[j]
				o, ok := pairKey[k]
				if !ok {
					o = &Overlap{FeatureA: owners[i], FeatureB: owners[j]}
					pairKey[k] = o
					keys = append(keys, k)
				}
				o.Paths = append(o.Paths, p)
			}
		}
	}
	sort.Strings(keys)
	out := make([]Overlap, 0, len(keys))
	for _, k := range keys {
		o := pairKey[k]
		sort.Strings(o.Paths)
		out = append(out, *o)
	}
	return out
}

func sortedPaths(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
