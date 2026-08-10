package credential

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvLookup returns the value of an environment variable (empty when unset).
type EnvLookup func(name string) string

// FileInfo returns metadata for a path without following the final symlink.
type FileInfo func(path string) (os.FileInfo, error)

// Checker evaluates source readiness. Its stat, environment, and ownership
// hooks are injectable so tests can exercise all states without touching the
// real filesystem or process environment.
type Checker struct {
	LookupEnv EnvLookup
	Stat      FileInfo
}

// NewChecker returns a Checker backed by the real environment and filesystem.
// Lstat is deliberate: credential and parent-directory symlinks are rejected
// rather than silently validating their targets.
func NewChecker() *Checker {
	return &Checker{
		LookupEnv: os.Getenv,
		Stat:      os.Lstat,
	}
}

// Check computes readiness for a single entry. Secret values are never read:
// file sources are inspected through stat metadata only and env sources are
// checked for presence.
func (c *Checker) Check(e Entry) Check {
	switch e.Kind() {
	case KindFile:
		return c.checkFile(e)
	case KindEnv:
		return c.checkEnv(e)
	default:
		return Check{Entry: e, Status: Unavailable, Reason: "unsupported source kind"}
	}
}

// Doctor evaluates every manifest entry.
func Doctor(m *Manifest, c *Checker) []Check {
	checks := make([]Check, 0, len(m.Entries))
	for _, e := range m.Entries {
		checks = append(checks, c.Check(e))
	}
	return checks
}

func (c *Checker) checkEnv(e Entry) Check {
	if e.Ref() == "" {
		return Check{Entry: e, Status: Unavailable, Reason: "environment variable name is empty"}
	}
	if c.LookupEnv(e.Ref()) == "" {
		return Check{Entry: e, Status: Unavailable, Reason: "environment variable is not set"}
	}
	return Check{Entry: e, Status: Ready}
}

func (c *Checker) checkFile(e Entry) Check {
	if !filePolicySupported() {
		return Check{Entry: e, Status: Unsupported, Reason: "file credential policy is unsupported on this platform"}
	}

	ref := e.Ref()
	if strings.HasPrefix(ref, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return Check{Entry: e, Status: Unavailable, Reason: "cannot resolve home directory"}
		}
		ref = filepath.Join(home, ref[2:])
	}

	info, err := c.Stat(ref)
	if err != nil {
		if os.IsNotExist(err) {
			return Check{Entry: e, Status: Missing, Reason: "credential file does not exist"}
		}
		return Check{Entry: e, Status: Unavailable, Reason: "credential file cannot be inspected"}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Check{Entry: e, Status: Unsafe, Reason: "credential source is a symlink"}
	}
	if info.IsDir() {
		return Check{Entry: e, Status: Unsafe, Reason: "credential source is a directory, not a file"}
	}

	if status, reason := c.checkAncestors(ref); status != "" {
		return Check{Entry: e, Status: status, Reason: reason}
	}
	if reason := fileSafety(info, true); reason != "" {
		return Check{Entry: e, Status: Unsafe, Reason: reason}
	}
	return Check{Entry: e, Status: Ready}
}

// checkAncestors inspects every directory component with Lstat semantics so a
// higher-level symlink cannot redirect validation outside the intended tree.
// Only the immediate parent is subject to the strict 0700 directory policy.
func (c *Checker) checkAncestors(path string) (Status, string) {
	immediate := filepath.Dir(path)
	for current := immediate; ; current = filepath.Dir(current) {
		info, err := c.Stat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return Missing, "credential directory does not exist"
			}
			return Unavailable, "credential directory cannot be inspected"
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return Unsafe, "credential directory is a symlink"
		}
		if current == immediate {
			if reason := fileSafety(info, false); reason != "" {
				return Unsafe, reason
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return "", ""
}

// fileSafety checks ownership and permission policy. Files require mode 0600
// (owner read/write only); directories require mode 0700 (owner read/write/execute).
// Ownership must match the current euid when the platform reports it.
func fileSafety(info os.FileInfo, isFile bool) string {
	if owner, ok := ownerOf(info); ok {
		if euid := currentEUID(); euid >= 0 && owner.uid != euid {
			return "credential source is not owned by the current user"
		}
	}

	if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return "credential source has unsafe special permission bits"
	}

	mode := info.Mode().Perm()
	if isFile {
		if mode != 0o600 {
			return "credential file mode must be 0600"
		}
	} else if mode != 0o700 {
		return "credential directory mode must be 0700"
	}
	return ""
}
