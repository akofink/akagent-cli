package output

import (
	"encoding/json"
	"math"
	"testing"
)

// estimateTokens approximates model token counts for English-heavy text as
// ceil(bytes/4), a common heuristic for the Claude/GPT tokenizers. It is a
// reproducible proxy, not an exact tokenizer count; results are tied to these
// sample-specific schemas and make no universal claim. See docs/toon.md.
func estimateTokens(s string) int {
	return int(math.Ceil(float64(len(s)) / 4))
}

// TestTokenMeasurement records the TOON-vs-compact-JSON token footprint for
// representative schemas the CLI currently emits. It is reproducible via:
//
//	go test ./internal/output -run TestTokenMeasurement -v
//
// The assertion keeps TOON from ever costing more tokens than compact JSON for
// these samples, and the logged ratios are the recorded measurements.
func TestTokenMeasurement(t *testing.T) {
	samples := map[string]any{
		"home view": map[string]any{
			"bin":         "~/bin/akagent",
			"description": "Orchestrate local coding agents through tmux and Git worktrees",
			"tasks":       []string{},
			"help": []string{
				"Run `akagent id generate` to create a task ID",
				"Run `akagent update` to update from the local source checkout",
				"Run `akagent worker inspect` to inspect the local worker",
			},
		},
		"worker inspect": map[string]any{
			"worker": map[string]any{
				"id":               "local",
				"protocol_version": 1,
				"architecture":     "arm64",
				"operating_system": "linux",
				"features":         []string{"tmux", "git-worktree"},
			},
		},
		"structured error": map[string]any{
			"error": map[string]any{
				"category":  "conflict",
				"message":   "Task inputs conflict with the existing task",
				"retryable": false,
				"recovery":  "akagent task inspect <task-id> --full",
			},
		},
		"tabular task list": map[string]any{
			"tasks": []map[string]any{
				{"id": "019f", "title": "Fix bootstrap", "status": "active"},
				{"id": "01a0", "title": "Review cleanup", "status": "waiting"},
			},
		},
	}

	totalToon, totalJSON := 0, 0
	t.Logf("%-18s %8s %8s %8s %7s", "sample", "jsonB", "toonB", "jsonT", "toonT")
	for name, value := range samples {
		toon, err := Encode(value)
		if err != nil {
			t.Fatalf("Encode(%s): %v", name, err)
		}
		compact, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("json.Marshal(%s): %v", name, err)
		}
		jTok, tTok := estimateTokens(string(compact)), estimateTokens(toon)
		totalToon += tTok
		totalJSON += jTok
		t.Logf("%-18s %8d %8d %8d %7d", name, len(compact), len(toon), jTok, tTok)
		if tTok > jTok {
			t.Errorf("%s: TOON token estimate %d > compact JSON %d (no savings)", name, tTok, jTok)
		}
	}

	ratio := float64(totalToon) / float64(totalJSON)
	t.Logf("tokens toon/json for these samples: %d/%d = %.0f%%", totalToon, totalJSON, ratio*100)
	// Recorded sample-specific result; the assertion only guards against a
	// regression to TOON ever costing more than compact JSON here.
	if totalToon > totalJSON {
		t.Errorf("aggregate TOON tokens %d exceed compact JSON %d", totalToon, totalJSON)
	}
}
