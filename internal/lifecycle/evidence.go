package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/akofink/akagent-cli/internal/store"
)

const (
	EvidenceSourceNativeReference = "native_reference"
	EvidenceRetentionMetadata     = "metadata"
	EvidenceRedactionMetadataOnly = "metadata_only"

	EvidenceStateAvailable   = "available"
	EvidenceStateUnavailable = "unavailable"
	EvidenceStateUnknown     = "unknown"
	EvidenceStateRecorded    = "recorded"

	EvidenceClassObserved    = "observed"
	EvidenceClassRecorded    = "recorded"
	EvidenceClassUnavailable = "unavailable"
)

// EvidenceCapture is the provider-neutral Phase 0 view of an execution session
// reference. It describes only metadata and safe local reference observations.
// It never contains provider transcript content, prompt text, terminal output,
// credentials, environment values, shell history, or raw derived artifacts.
type EvidenceCapture struct {
	CaptureID         string
	ExecutionID       string
	SourceKind        string
	Provider          string
	ProviderSessionID string
	State             string
	EvidenceClass     string
	Coverage          []string
	ArtifactReference string
	ArtifactState     string
	RedactionPolicy   string
	RetentionClass    string
	ErrorCategory     string
	Recovery          string
}

// EvidenceSummary explains whether Phase 0 evidence exists for an execution.
type EvidenceSummary struct {
	TaskID        string
	ExecutionID   string
	State         string
	EvidenceClass string
	Reason        string
}

// ListExecutionEvidence derives a deterministic read-only evidence view from
// existing provider-neutral session references. It validates only the shape and
// availability of local reference paths and never opens provider-owned files.
func (m *Manager) ListExecutionEvidence(taskID, executionID string) (EvidenceSummary, []EvidenceCapture, error) {
	execution, err := m.InspectExecution(taskID, executionID)
	if err != nil {
		return EvidenceSummary{}, nil, err
	}
	captures := evidenceFromSessionReferences(execution)
	summary := EvidenceSummary{TaskID: taskID, ExecutionID: execution.ID}
	if len(captures) == 0 {
		summary.State = EvidenceStateUnavailable
		summary.EvidenceClass = EvidenceClassUnavailable
		summary.Reason = "no_session_references"
		return summary, captures, nil
	}
	summary.State = EvidenceStateRecorded
	summary.EvidenceClass = EvidenceClassRecorded
	summary.Reason = "session_references_recorded"
	return summary, captures, nil
}

// InspectExecutionEvidence returns one Phase 0 evidence capture by its stable
// capture ID. Inspection is read-only and never parses provider artifacts.
func (m *Manager) InspectExecutionEvidence(taskID, executionID, captureID string) (EvidenceCapture, error) {
	_, captures, err := m.ListExecutionEvidence(taskID, executionID)
	if err != nil {
		return EvidenceCapture{}, err
	}
	for _, capture := range captures {
		if capture.CaptureID == captureID {
			return capture, nil
		}
	}
	return EvidenceCapture{}, &store.Error{Kind: store.KindNotFound, Message: fmt.Sprintf("No evidence capture %s found for execution %s", captureID, executionID), Recovery: fmt.Sprintf("Run `akagent task execution evidence list %s %s`", taskID, executionID)}
}

func evidenceFromSessionReferences(execution store.Execution) []EvidenceCapture {
	references := append([]store.SessionReference(nil), execution.SessionReferences...)
	sort.SliceStable(references, func(i, j int) bool {
		return sessionReferenceSortKey(references[i]) < sessionReferenceSortKey(references[j])
	})
	captures := make([]EvidenceCapture, 0, len(references))
	for _, reference := range references {
		captures = append(captures, evidenceFromSessionReference(execution.ID, reference))
	}
	return captures
}

func sessionReferenceSortKey(reference store.SessionReference) string {
	return reference.Tool + "\x00" + reference.SessionID + "\x00" + reference.ReferencePath
}

func evidenceFromSessionReference(executionID string, reference store.SessionReference) EvidenceCapture {
	capture := EvidenceCapture{
		CaptureID:         evidenceCaptureID(executionID, reference),
		ExecutionID:       executionID,
		SourceKind:        EvidenceSourceNativeReference,
		Provider:          reference.Tool,
		ProviderSessionID: reference.SessionID,
		State:             EvidenceStateUnknown,
		EvidenceClass:     EvidenceClassRecorded,
		Coverage:          []string{"native_reference"},
		ArtifactReference: reference.ReferencePath,
		ArtifactState:     EvidenceStateUnknown,
		RedactionPolicy:   EvidenceRedactionMetadataOnly,
		RetentionClass:    EvidenceRetentionMetadata,
	}
	if reference.ReferencePath == "" {
		capture.ErrorCategory = "no_artifact_reference"
		capture.Recovery = "Record a provider-owned local reference path when the integration can safely identify one"
		return capture
	}
	info, err := os.Lstat(reference.ReferencePath)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular() && info.Mode().Perm()&0444 != 0:
		capture.State = EvidenceStateAvailable
		capture.EvidenceClass = EvidenceClassObserved
		capture.ArtifactState = EvidenceStateAvailable
		capture.Coverage = []string{"native_reference", "local_reference"}
		return capture
	case err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular():
		capture.State = EvidenceStateUnavailable
		capture.EvidenceClass = EvidenceClassUnavailable
		capture.ArtifactState = EvidenceStateUnavailable
		capture.ErrorCategory = "artifact_unreadable"
		capture.Recovery = "Restore read access through the provider or operator's local permissions"
		return capture
	case err != nil && errors.Is(err, os.ErrNotExist):
		capture.State = EvidenceStateUnavailable
		capture.EvidenceClass = EvidenceClassUnavailable
		capture.ArtifactState = EvidenceStateUnavailable
		capture.ErrorCategory = "artifact_missing"
		capture.Recovery = "Use the provider-native resume or discovery flow to verify whether the session is still available"
		return capture
	case err != nil:
		capture.State = EvidenceStateUnavailable
		capture.EvidenceClass = EvidenceClassUnavailable
		capture.ArtifactState = EvidenceStateUnavailable
		capture.ErrorCategory = "artifact_unavailable"
		capture.Recovery = "Check local permissions or provider cleanup state outside akagent"
		return capture
	default:
		capture.State = EvidenceStateUnavailable
		capture.EvidenceClass = EvidenceClassUnavailable
		capture.ArtifactState = EvidenceStateUnavailable
		capture.ErrorCategory = "artifact_not_regular_file"
		capture.Recovery = "Record a provider-owned regular local session reference file"
		return capture
	}
}

func evidenceCaptureID(executionID string, reference store.SessionReference) string {
	digest := sha256.Sum256([]byte(executionID + "\x00" + sessionReferenceSortKey(reference)))
	return "ref-" + strings.ToLower(hex.EncodeToString(digest[:8]))
}
