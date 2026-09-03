package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	"github.com/GrayCodeAI/graycode-cli/internal/graphjournal"
)

// JournalHarnessReport records a privacy-safe quality observation of a harness report into Graycode's execution graph.
func JournalHarnessReport(report *HarnessReport, sessionID string) error {
	if report == nil {
		return nil
	}

	if sessionID == "" {
		sessionID = fmt.Sprintf("harness-%d", time.Now().Unix())
	}

	hash := sha256.Sum256([]byte(report.TargetPath))
	targetSHA := hex.EncodeToString(hash[:])

	maxSev := "INFO"
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical || f.Severity == SeverityHigh {
			maxSev = string(f.Severity)
			break
		}
		if f.Severity == SeverityMedium {
			maxSev = "MEDIUM"
		}
	}

	// 1. Record Verification Summary
	_ = graphjournal.AppendVerification(
		sessionID,
		"",
		"harness_review",
		report.OverallScore < 50,
		len(report.Findings),
		maxSev,
		targetSHA,
		report.GeneratedAt,
	)

	// 2. Record Quality Graph Subgraph
	now := report.GeneratedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	prov := graphcontracts.Provenance{
		Producer: "harness_evaluator",
		Version:  "1.0.0",
	}

	nodes := []graphcontracts.Node{
		{
			ID:         "harness_overall_score",
			Kind:       graphcontracts.NodeQuality,
			CreatedAt:  now,
			Provenance: prov,
			Attributes: map[string]string{
				"score":  fmt.Sprintf("%d", report.OverallScore),
				"status": report.OverallStatus,
			},
		},
	}

	for dim, ds := range report.Dimensions {
		nodes = append(nodes, graphcontracts.Node{
			ID:         fmt.Sprintf("dimension_%s", string(dim)),
			Kind:       graphcontracts.NodeQuality,
			CreatedAt:  now,
			Provenance: prov,
			Attributes: map[string]string{
				"dimension": string(dim),
				"score":     fmt.Sprintf("%d", ds.Score),
				"state":     string(ds.State),
			},
		})
	}

	return graphjournal.AppendQualityGraph(
		sessionID,
		"",
		"harness_review",
		"harness_evaluator",
		nodes,
		nil,
		nil,
		now,
	)
}
