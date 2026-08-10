// Package credential models the local, non-secret credential manifest and
// readiness checks.
//
// The manifest lives under XDG configuration and holds only source references
// and policy, never secret values. This package never reads credential file
// contents or environment variable values for output; readiness is computed
// from metadata (stat, ownership, permissions) and environment presence only.
package credential

// Status is the readiness state of a credential source.
type Status string

const (
	// Ready means the source is present and safe to use.
	Ready Status = "ready"
	// Missing means a file source does not exist.
	Missing Status = "missing"
	// Unsafe means a file source exists but ownership or permissions violate policy.
	Unsafe Status = "unsafe"
	// Unavailable means an environment source is absent or cannot be inspected.
	Unavailable Status = "unavailable"
	// Unsupported means this platform cannot apply the local file policy.
	Unsupported Status = "unsupported"
)

const (
	// KindFile is a file-backed source reference: "file:<path>".
	KindFile = "file"
	// KindEnv is an environment-variable source reference: "env:<VAR>".
	KindEnv = "env"
)

// Entry is one non-secret manifest row describing a credential source.
type Entry struct {
	ID          string
	Type        string
	Source      string // full reference, e.g. "file:/abs/path" or "env:VAR"
	RequiredFor string // capability; empty means an optional credential
}

// Required reports whether the entry is required. Optional entries only warn
// when their source is unavailable.
func (e Entry) Required() bool {
	return e.RequiredFor != ""
}

// Kind returns the parsed source kind ("file" or "env") or an empty string when
// the reference has no recognized "kind:" prefix.
func (e Entry) Kind() string {
	for i := 0; i < len(e.Source); i++ {
		if e.Source[i] == ':' {
			return e.Source[:i]
		}
	}
	return ""
}

// Ref returns the reference payload after the "kind:" prefix (a file path or an
// environment variable name). The value is non-secret.
func (e Entry) Ref() string {
	for i := 0; i < len(e.Source); i++ {
		if e.Source[i] == ':' {
			return e.Source[i+1:]
		}
	}
	return ""
}

// Manifest is the versioned, parsed credential manifest.
type Manifest struct {
	Version int
	Entries []Entry
}

// Check is the readiness result for a single entry. Reason is a non-secret
// summary suitable for warnings and errors.
type Check struct {
	Entry  Entry
	Status Status
	Reason string
}
