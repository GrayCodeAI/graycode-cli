package harness

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// RenderJSON serializes the harness evaluation report to formatted JSON bytes.
func RenderJSON(report *HarnessReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// RenderMarkdown formats the report into a detailed GitHub-flavored Markdown document.
func RenderMarkdown(report *HarnessReport) string {
	var sb strings.Builder

	sb.WriteString("# 🦅 Hawk Agent Harness Review Report\n\n")
	sb.WriteString(fmt.Sprintf("**Target Workspace:** `%s`  \n", report.TargetPath))
	sb.WriteString(fmt.Sprintf("**Generated At:** %s  \n", report.GeneratedAt.Format(time.RFC1123)))
	sb.WriteString(fmt.Sprintf("**Overall Harness Health Score:** **%d / 100** (%s)\n\n", report.OverallScore, report.OverallStatus))

	sb.WriteString("## Executive Summary\n\n")
	sb.WriteString(report.Summary + "\n\n")

	sb.WriteString("## 📊 Agent Work Loop Dimensions\n\n")
	sb.WriteString("| Dimension | Score | Status | Summary |\n")
	sb.WriteString("| :--- | :---: | :---: | :--- |\n")

	orderedDims := []Dimension{
		DimensionFeedforward,
		DimensionFeedback,
		DimensionTaskUnderstanding,
		DimensionStepPlanning,
		DimensionVerification,
	}

	for _, dim := range orderedDims {
		if ds, ok := report.Dimensions[dim]; ok {
			sb.WriteString(fmt.Sprintf("| **%s** | **%d%%** | `%s` | %s |\n", ds.Dimension, ds.Score, ds.State, ds.Summary))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## 🔍 Detected Harness Assets\n\n")
	sb.WriteString(fmt.Sprintf("- **AGENTS.md**: %v\n", report.Assets.AgentsMD))
	if report.Assets.AgentsMDPath != "" {
		sb.WriteString(fmt.Sprintf("  - *Path*: `%s`\n", report.Assets.AgentsMDPath))
	}
	sb.WriteString(fmt.Sprintf("- **ZERO.md**: %v\n", report.Assets.ZeroMD))
	sb.WriteString(fmt.Sprintf("- **Custom Skills**: %d skills detected (%s)\n", len(report.Assets.Skills), strings.Join(report.Assets.Skills, ", ")))
	sb.WriteString(fmt.Sprintf("- **Linters**: %s\n", strings.Join(report.Assets.Linters, ", ")))
	sb.WriteString(fmt.Sprintf("- **Test Runners**: %s\n", strings.Join(report.Assets.TestRunners, ", ")))
	sb.WriteString(fmt.Sprintf("- **Hooks**: %s\n", strings.Join(report.Assets.Hooks, ", ")))
	sb.WriteString(fmt.Sprintf("- **Autonomy Policy**: `%s` tier (Sandbox: `%s`)\n\n", report.Assets.AutonomyTier, report.Assets.SandboxPolicy))

	sb.WriteString("## ⚠️ Prioritized Findings & Repair Recommendations\n\n")
	if len(report.Findings) == 0 {
		sb.WriteString("✨ **No critical harness findings detected!** Your project is well-configured for AI agent workflows.\n")
	} else {
		for i, f := range report.Findings {
			sb.WriteString(fmt.Sprintf("### %d. [%s] %s (%s)\n\n", i+1, f.Severity, f.Title, f.Dimension))
			sb.WriteString(fmt.Sprintf("- **Description:** %s\n", f.Description))
			sb.WriteString(fmt.Sprintf("- **Impact:** %s\n", f.Impact))
			sb.WriteString(fmt.Sprintf("- **Evidence:** `%s` (`%s`)\n", f.EvidenceSource, f.EvidenceState))
			sb.WriteString(fmt.Sprintf("- **Expected Outcome:** %s\n", f.ExpectedOutcome))
			sb.WriteString(fmt.Sprintf("- **Scoped Repair:** %s\n", f.ScopedRepair))
			sb.WriteString(fmt.Sprintf("- **Validation Command:** `%s`\n\n", f.ValidationRoute))
		}
	}

	return sb.String()
}

// RenderHTML formats the report into a self-contained HTML dashboard.
func RenderHTML(report *HarnessReport) string {
	var sb strings.Builder

	sb.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Hawk Harness Evaluation Report</title>
    <style>
        :root {
            --bg-color: #0f172a;
            --card-bg: #1e293b;
            --text-main: #f8fafc;
            --text-sub: #94a3b8;
            --accent-blue: #38bdf8;
            --accent-green: #4ade80;
            --accent-yellow: #facc15;
            --accent-red: #f87171;
            --border-color: #334155;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            background-color: var(--bg-color);
            color: var(--text-main);
            margin: 0;
            padding: 40px 20px;
        }
        .container {
            max-width: 1000px;
            margin: 0 auto;
        }
        .header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            border-bottom: 2px solid var(--border-color);
            padding-bottom: 20px;
            margin-bottom: 30px;
        }
        .title-area h1 {
            margin: 0 0 10px 0;
            font-size: 28px;
            color: var(--accent-blue);
        }
        .meta-info {
            color: var(--text-sub);
            font-size: 14px;
        }
        .score-badge {
            background-color: var(--card-bg);
            border: 2px solid var(--accent-blue);
            border-radius: 12px;
            padding: 15px 25px;
            text-align: center;
        }
        .score-num {
            font-size: 36px;
            font-weight: bold;
            color: var(--accent-green);
        }
        .score-label {
            font-size: 12px;
            text-transform: uppercase;
            letter-spacing: 1px;
            color: var(--text-sub);
        }
        .card {
            background: var(--card-bg);
            border-radius: 12px;
            padding: 24px;
            margin-bottom: 24px;
            border: 1px solid var(--border-color);
        }
        .card h2 {
            margin-top: 0;
            font-size: 20px;
            color: var(--accent-blue);
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 15px;
        }
        th, td {
            padding: 12px 16px;
            text-align: left;
            border-bottom: 1px solid var(--border-color);
        }
        th {
            background-color: rgba(255, 255, 255, 0.05);
            color: var(--text-sub);
            font-size: 13px;
            text-transform: uppercase;
        }
        .badge {
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: bold;
        }
        .badge-CRITICAL { background: var(--accent-red); color: #000; }
        .badge-HIGH { background: #fb923c; color: #000; }
        .badge-MEDIUM { background: var(--accent-yellow); color: #000; }
        .badge-LOW { background: var(--accent-blue); color: #000; }
        .finding-item {
            border-left: 4px solid var(--accent-blue);
            background: rgba(255, 255, 255, 0.02);
            padding: 16px;
            margin-bottom: 16px;
            border-radius: 4px;
        }
        .finding-title {
            font-size: 16px;
            font-weight: bold;
            margin-bottom: 8px;
        }
        .finding-meta {
            font-size: 13px;
            color: var(--text-sub);
            margin-bottom: 8px;
        }
        code {
            background: rgba(255, 255, 255, 0.1);
            padding: 2px 6px;
            border-radius: 4px;
            font-family: monospace;
        }
    </style>
</head>
<body>
<div class="container">
    <div class="header">
        <div class="title-area">
            <h1>🦅 Hawk Agent Harness Review</h1>
            <div class="meta-info">Workspace: <code>` + report.TargetPath + `</code> | Generated: ` + report.GeneratedAt.Format(time.RFC1123) + `</div>
        </div>
        <div class="score-badge">
            <div class="score-num">` + fmt.Sprintf("%d", report.OverallScore) + `/100</div>
            <div class="score-label">` + report.OverallStatus + `</div>
        </div>
    </div>

    <div class="card">
        <h2>Executive Summary</h2>
        <p>` + report.Summary + `</p>
    </div>

    <div class="card">
        <h2>Agent Work Loop Dimensions</h2>
        <table>
            <thead>
                <tr>
                    <th>Dimension</th>
                    <th>Score</th>
                    <th>Status</th>
                    <th>Summary</th>
                </tr>
            </thead>
            <tbody>`)

	orderedDims := []Dimension{
		DimensionFeedforward,
		DimensionFeedback,
		DimensionTaskUnderstanding,
		DimensionStepPlanning,
		DimensionVerification,
	}

	for _, dim := range orderedDims {
		if ds, ok := report.Dimensions[dim]; ok {
			sb.WriteString(fmt.Sprintf(`
                <tr>
                    <td><strong>%s</strong></td>
                    <td><strong>%d%%</strong></td>
                    <td><code>%s</code></td>
                    <td>%s</td>
                </tr>`, ds.Dimension, ds.Score, ds.State, ds.Summary))
		}
	}

	sb.WriteString(`
            </tbody>
        </table>
    </div>

    <div class="card">
        <h2>Prioritized Findings & Repair Plans</h2>`)

	if len(report.Findings) == 0 {
		sb.WriteString(`<p>✨ <strong>No critical findings!</strong> Workspace harness is well-configured.</p>`)
	} else {
		for _, f := range report.Findings {
			sb.WriteString(fmt.Sprintf(`
        <div class="finding-item">
            <div class="finding-title"><span class="badge badge-%s">%s</span> %s</div>
            <div class="finding-meta">Dimension: <strong>%s</strong> | Evidence Source: <code>%s</code></div>
            <p><strong>Impact:</strong> %s</p>
            <p><strong>Scoped Repair Plan:</strong> %s</p>
            <p><strong>Validation Command:</strong> <code>%s</code></p>
        </div>`, f.Severity, f.Severity, f.Title, f.Dimension, f.EvidenceSource, f.Impact, f.ScopedRepair, f.ValidationRoute))
		}
	}

	sb.WriteString(`
    </div>
</div>
</body>
</html>`)

	return sb.String()
}
