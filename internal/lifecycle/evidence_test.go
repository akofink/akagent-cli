package lifecycle

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

func TestExecutionEvidenceDerivesMetadataOnlyStatesFromSessionReferences(t *testing.T) {
	manager := newEvidenceManager(t)
	if _, err := manager.Create(CreateRequest{ID: "evidence-task", Title: "Evidence task"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateExecution("evidence-task", ExecutionRequest{ID: "evidence-exec", Target: "pi", Command: "pi"}); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(artifact, []byte("prompt text and credential-shaped content must never be inspected"), 0o600); err != nil {
		t.Fatal(err)
	}
	available := store.SessionReference{Tool: "pi", SessionID: "pi-session", ReferencePath: artifact}
	unknown := store.SessionReference{Tool: "codex", SessionID: "codex-session"}
	missingPath := filepath.Join(t.TempDir(), "missing.jsonl")
	if err := os.WriteFile(missingPath, []byte("transient provider-owned reference"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := store.SessionReference{Tool: "claude", SessionID: "claude-session", ReferencePath: missingPath}
	for _, reference := range []store.SessionReference{available, unknown, missing} {
		if _, err := manager.AddExecutionSessionReference("evidence-task", "evidence-exec", reference); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(missingPath); err != nil {
		t.Fatal(err)
	}

	summary, captures, err := manager.ListExecutionEvidence("evidence-task", "evidence-exec")
	if err != nil {
		t.Fatal(err)
	}
	if summary.State != EvidenceStateRecorded || summary.EvidenceClass != EvidenceClassRecorded || summary.Reason != "session_references_recorded" {
		t.Fatalf("summary = %#v, want recorded references", summary)
	}
	if len(captures) != 3 {
		t.Fatalf("captures = %#v, want three session-reference captures", captures)
	}
	got := map[string]EvidenceCapture{}
	for _, capture := range captures {
		got[capture.Provider] = capture
	}
	if got["pi"].State != EvidenceStateAvailable || got["pi"].EvidenceClass != EvidenceClassObserved || got["pi"].ArtifactState != EvidenceStateAvailable || got["pi"].ErrorCategory != "" {
		t.Fatalf("available capture = %#v", got["pi"])
	}
	if got["codex"].State != EvidenceStateUnknown || got["codex"].EvidenceClass != EvidenceClassRecorded || got["codex"].ErrorCategory != "no_artifact_reference" {
		t.Fatalf("unknown capture = %#v", got["codex"])
	}
	if got["claude"].State != EvidenceStateUnavailable || got["claude"].EvidenceClass != EvidenceClassUnavailable || got["claude"].ErrorCategory != "artifact_missing" {
		t.Fatalf("missing capture = %#v", got["claude"])
	}
	if got["pi"].CaptureID != evidenceCaptureID("evidence-exec", available) || got["codex"].CaptureID != evidenceCaptureID("evidence-exec", unknown) || got["claude"].CaptureID != evidenceCaptureID("evidence-exec", missing) {
		t.Fatalf("capture IDs = %#v, want stable IDs from provider-neutral references", got)
	}
}

func TestExecutionEvidenceReportsAbsentReferencesDistinctly(t *testing.T) {
	manager := newEvidenceManager(t)
	if _, err := manager.Create(CreateRequest{ID: "empty-evidence-task", Title: "Empty evidence task"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateExecution("empty-evidence-task", ExecutionRequest{ID: "empty-evidence-exec", Target: "shell", Command: "/bin/sh"}); err != nil {
		t.Fatal(err)
	}

	summary, captures, err := manager.ListExecutionEvidence("empty-evidence-task", "empty-evidence-exec")
	if err != nil {
		t.Fatal(err)
	}
	if summary.State != EvidenceStateUnavailable || summary.EvidenceClass != EvidenceClassUnavailable || summary.Reason != "no_session_references" {
		t.Fatalf("summary = %#v, want explicit absent-reference evidence", summary)
	}
	if !reflect.DeepEqual(captures, []EvidenceCapture{}) {
		t.Fatalf("captures = %#v, want empty slice", captures)
	}
	if _, err := manager.InspectExecutionEvidence("empty-evidence-task", "empty-evidence-exec", "missing"); !store.IsKind(err, store.KindNotFound) {
		t.Fatalf("InspectExecutionEvidence missing = %v, want not found", err)
	}
}

func newEvidenceManager(t *testing.T) *Manager {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	state, err := store.OpenAt(root)
	if err != nil {
		t.Fatal(err)
	}
	return New(state)
}
