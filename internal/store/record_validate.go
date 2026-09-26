package store

import (
	"fmt"
	"path/filepath"
	"strings"
)

func validateExternalTaskRequest(request ExternalTaskRequest) error {
	if err := validateCallerID(request.CallerID); err != nil {
		return err
	}
	if err := validateOperationID(request.OperationID); err != nil {
		return err
	}
	if strings.TrimSpace(request.Title) == "" || strings.ContainsAny(request.Title, "\r\n\x00") || len(request.Title) > 4096 {
		return newError(KindUsage, "external task title must be a bounded single line", "Provide a non-secret task title")
	}
	return nil
}

func validateExternalResourceRequest(request ExternalResourceRequest) error {
	if err := validateOperationID(request.OperationID); err != nil {
		return err
	}
	if err := validateResourceID(request.ID); err != nil {
		return err
	}
	if err := validateCallerID(request.CallerID); err != nil {
		return err
	}
	if strings.TrimSpace(request.Repository) == "" || strings.TrimSpace(request.Branch) == "" || strings.TrimSpace(request.BaseRevision) == "" || strings.TrimSpace(request.Head) == "" {
		return newError(KindUsage, "external resource repository, branch, base, and head are required", "Provide caller-declared repository identity and revisions")
	}
	for _, value := range []string{request.Repository, request.Branch, request.BaseRevision, request.Head} {
		if strings.ContainsAny(value, "\r\n\x00") || len(value) > 4096 {
			return newError(KindUsage, "external resource identity values must be bounded single lines", "Retry with redacted non-secret identity values")
		}
	}
	if !filepath.IsAbs(request.WorktreePath) || strings.ContainsAny(request.WorktreePath, "\r\n\x00") || len(request.WorktreePath) > 4096 {
		return newError(KindUsage, "external resource worktree must be a bounded absolute path", "Provide a bounded absolute referenced worktree path; it need not exist")
	}
	if len(request.Metadata) > 64 {
		return newError(KindUsage, "external resource metadata has too many entries", "Provide at most 64 non-secret metadata entries")
	}
	for key, value := range request.Metadata {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\r\n\x00") || len(key) > 256 || strings.ContainsAny(value, "\r\n\x00") || len(value) > 4096 {
			return newError(KindUsage, "external resource metadata keys and values must be bounded single lines", "Retry with bounded non-secret metadata")
		}
	}
	return nil
}

func validateExternalExecutionRequest(request ExternalExecutionRequest) error {
	if err := validateOperationID(request.OperationID); err != nil {
		return err
	}
	if err := validateExecutionID(request.ID); err != nil {
		return err
	}
	if err := validateCallerID(request.CallerID); err != nil {
		return err
	}
	if err := validateResourceID(request.ResourceID); err != nil {
		return err
	}
	if request.PredecessorID != "" {
		if err := validateExecutionID(request.PredecessorID); err != nil {
			return err
		}
		if request.PredecessorID == request.ID {
			return newError(KindConflict, "external execution cannot be its own predecessor", "Choose a prior execution ID")
		}
	}
	if len(request.SessionReferences) > 32 {
		return newError(KindUsage, "external execution has too many session references", "Provide at most 32 provider-neutral session references")
	}
	for _, reference := range request.SessionReferences {
		if err := validateSessionReferenceShape(reference); err != nil {
			return err
		}
	}
	return nil
}

func validateOperationID(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") || len(value) > 256 {
		return newError(KindUsage, "operation ID must be a non-empty bounded single line", "Provide a stable per-operation idempotency key")
	}
	return nil
}

func validateCallerID(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") || len(value) > 256 {
		return newError(KindUsage, "caller ID must be a non-empty bounded single line", "Provide a stable non-secret caller ID")
	}
	return nil
}

func validateExternalObservation(observation ExternalObservation) error {
	if strings.TrimSpace(observation.Source) == "" || strings.TrimSpace(observation.HostID) == "" || strings.TrimSpace(observation.BootID) == "" || observation.ObservedAt.IsZero() {
		return newError(KindUsage, "external observations require source, observed time, host ID, and boot ID", "Provide complete observation provenance")
	}
	for _, value := range []string{observation.Source, observation.HostID, observation.BootID, observation.ProcessState, observation.Result, observation.Detail} {
		if strings.ContainsAny(value, "\r\n\x00") || len(value) > 4096 {
			return newError(KindUsage, "external observation values must be bounded single lines", "Retry with redacted non-secret observation values")
		}
	}
	return nil
}

func validateExternalCompletion(completion ExternalCompletion) error {
	if completion.DeclaredAt.IsZero() {
		return newError(KindUsage, "external completion declaration time is required", "Repair the external completion record")
	}
	return validateCompletion(completion.Contract, completion.Result, completion.CallerID)
}

func validateCompletion(contract, resultValue, callerID string) error {
	if err := validateCallerID(callerID); err != nil {
		return err
	}
	if strings.TrimSpace(contract) == "" || strings.TrimSpace(resultValue) == "" || strings.ContainsAny(contract+resultValue, "\r\n\x00") || len(contract) > 4096 || len(resultValue) > 4096 {
		return newError(KindUsage, "completion contract and result are required bounded single-line values", "Declare completion against a named external contract")
	}
	return nil
}

func sameExternalResourceBinding(current Resource, request ExternalResourceRequest) bool {
	metadataSame := request.Metadata == nil || sameStringMap(current.Metadata, mergeStringMap(current.Metadata, request.Metadata))
	return current.Repository == request.Repository && current.Branch == request.Branch && current.BaseRevision == request.BaseRevision && current.WorktreePath == request.WorktreePath && current.Git.Head == request.Head && metadataSame
}

func validateExternalTaskCaller(manifest Manifest, taskID, callerID string) error {
	if manifest.Provenance != ProvenanceExternal {
		return externalOwnershipConflict("task", taskID)
	}
	if manifest.CallerID != callerID {
		return externalCallerConflict("task", taskID)
	}
	return nil
}

func validateExternalResourceCaller(resource Resource, callerID string) error {
	if resource.Provenance != ProvenanceExternal {
		return externalOwnershipConflict("resource", resource.ID)
	}
	if resource.CallerID != callerID {
		return externalCallerConflict("resource", resource.ID)
	}
	return nil
}

func validateExternalExecutionCaller(execution Execution, callerID string) error {
	if execution.Provenance != ProvenanceExternal {
		return externalOwnershipConflict("execution", execution.ID)
	}
	if execution.CallerID != callerID {
		return externalCallerConflict("execution", execution.ID)
	}
	return nil
}

func externalTaskTerminal(manifest Manifest) bool {
	return manifest.Lifecycle == "finished" || manifest.ExternalCompletion != nil || manifest.ArchiveState == "complete"
}

func externalCallerConflict(kind, id string) error {
	return newError(KindConflict, fmt.Sprintf("external %s %s belongs to another caller", kind, id), "Use the original stable caller ID or choose a new record ID")
}

func terminalMutationConflict(kind, id string) error {
	return newError(KindConflict, fmt.Sprintf("external %s %s is terminal and immutable", kind, id), "Use an explicit future reopen operation before changing a terminal record")
}

func expectedRevisionCheck(expected, current uint64, kind, id string) error {
	if expected != current {
		return revisionConflict(kind, id, current)
	}
	return nil
}

func revisionConflict(kind, id string, current uint64) error {
	return newError(KindConflict, fmt.Sprintf("%s %s revision conflict (current revision %d)", kind, id, current), "Inspect the record and retry with its current expected revision")
}

func externalOwnershipConflict(kind, id string) error {
	return newError(KindConflict, fmt.Sprintf("%s %s is not an externally declared record", kind, id), "Use the state-only record surface without changing legacy managed ownership")
}

func completionMatches(completion *ExternalCompletion, contract, result, callerID string) bool {
	return completion != nil && completion.Contract == contract && completion.Result == result && completion.CallerID == callerID
}
