package startup

import (
	"sync"
	"time"
)

type Phase struct {
	Name      string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
}

var (
	phases   []Phase
	phasesMu sync.Mutex
	start    = time.Now()
)

func MarkPhase(name string) {
	phasesMu.Lock()
	defer phasesMu.Unlock()
	phases = append(phases, Phase{
		Name:      name,
		StartTime: time.Now(),
	})
}

func EndPhase(name string) {
	phasesMu.Lock()
	defer phasesMu.Unlock()
	now := time.Now()
	for i := len(phases) - 1; i >= 0; i-- {
		if phases[i].Name == name && phases[i].EndTime.IsZero() {
			phases[i].EndTime = now
			phases[i].Duration = now.Sub(phases[i].StartTime)
			break
		}
	}
}

func GetPhases() []Phase {
	phasesMu.Lock()
	defer phasesMu.Unlock()
	result := make([]Phase, len(phases))
	copy(result, phases)
	return result
}

func TotalTime() time.Duration {
	return time.Since(start)
}

func PrintReport() {
	phasesMu.Lock()
	defer phasesMu.Unlock()

	println("\n=== Startup Profile ===")
	println("Total:", TotalTime())
	println()
	for _, p := range phases {
		if !p.EndTime.IsZero() {
			println(p.Name, ":", p.Duration.Round(time.Millisecond))
		}
	}
	println("========================")
}
