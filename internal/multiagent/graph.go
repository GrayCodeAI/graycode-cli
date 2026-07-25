package mission

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// RunWaves executes features in topological waves. Each inner slice contains the
// feature IDs that may run in parallel. Later waves never start until the
// current wave has joined, and remaining waves are left pending after a failure.
func (m *Mission) RunWaves(ctx context.Context, waves [][]string, workerFn WorkerFunc) error {
	m.mu.Lock()
	m.Status = StatusExecuting
	m.mu.Unlock()

	missionDir, err := m.ensureRunDir()
	if err != nil {
		return err
	}
	if err := m.persistPortableGraph(); err != nil {
		return err
	}

	index := make(map[string]*Feature, len(m.Features))
	for i := range m.Features {
		index[m.Features[i].ID] = &m.Features[i]
	}

	remaining := flattenWaveIDs(waves)
	for waveIndex, waveIDs := range waves {
		features := make([]*Feature, 0, len(waveIDs))
		for _, id := range waveIDs {
			feat, ok := index[id]
			if !ok {
				return fmt.Errorf("wave %d references unknown feature %q", waveIndex+1, id)
			}
			features = append(features, feat)
		}

		startedAt := time.Now()
		if err := m.runFeatureSet(ctx, workerFn, missionDir, features); err != nil {
			return err
		}
		remaining = subtractRemaining(remaining, waveIDs)

		completed, failed := classifyWave(features)
		if len(completed) > 0 {
			slices.Sort(completed)
		}
		if len(failed) > 0 {
			slices.Sort(failed)
		}
		blocked := []string(nil)
		if len(failed) > 0 {
			blocked = append(blocked, remaining...)
			slices.Sort(blocked)
		}

		m.mu.Lock()
		m.WaveJoins = append(m.WaveJoins, WaveJoin{
			Wave:         waveIndex + 1,
			FeatureIDs:   append([]string(nil), waveIDs...),
			CompletedIDs: completed,
			FailedIDs:    failed,
			BlockedIDs:   blocked,
			StartedAt:    startedAt,
			CompletedAt:  time.Now(),
			Summary:      summarizeWaveJoin(waveIndex+1, waveIDs, completed, failed, blocked),
		})
		m.mu.Unlock()
		if err := m.persistState(); err != nil {
			return err
		}
		if err := m.persistPortableGraph(); err != nil {
			return err
		}
		if len(failed) > 0 {
			break
		}
	}

	m.recomputeStatus()
	if err := m.persistState(); err != nil {
		return err
	}
	return m.persistPortableGraph()
}

func classifyWave(features []*Feature) (completed []string, failed []string) {
	for _, feat := range features {
		switch feat.Status {
		case FeatureCompleted:
			completed = append(completed, feat.ID)
		case FeatureFailed:
			failed = append(failed, feat.ID)
		}
	}
	return completed, failed
}

func flattenWaveIDs(waves [][]string) []string {
	total := 0
	for _, wave := range waves {
		total += len(wave)
	}
	out := make([]string, 0, total)
	for _, wave := range waves {
		out = append(out, wave...)
	}
	return out
}

func cloneExecutionWaves(waves [][]string) [][]string {
	if len(waves) == 0 {
		return nil
	}
	cloned := make([][]string, len(waves))
	for i, wave := range waves {
		cloned[i] = append([]string(nil), wave...)
	}
	return cloned
}

func subtractRemaining(remaining, executed []string) []string {
	if len(executed) == 0 {
		return remaining
	}
	remove := make(map[string]struct{}, len(executed))
	for _, id := range executed {
		remove[id] = struct{}{}
	}
	out := make([]string, 0, len(remaining))
	for _, id := range remaining {
		if _, ok := remove[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

func summarizeWaveJoin(wave int, featureIDs, completed, failed, blocked []string) string {
	parts := []string{
		fmt.Sprintf("wave %d joined %d feature(s)", wave, len(featureIDs)),
		fmt.Sprintf("%d completed", len(completed)),
		fmt.Sprintf("%d failed", len(failed)),
	}
	if len(blocked) > 0 {
		parts = append(parts, fmt.Sprintf("%d downstream blocked", len(blocked)))
	}
	return strings.Join(parts, ", ")
}
