package credential

import (
	"errors"
	"sort"
	"strings"
)

// BuildEnvironment returns a minimal environment for a managed process.
// Ambient values are copied only from the safe runtime allowlist. Requested
// environment credentials are read only after capability checks and injected
// under their manifest variable names.
func BuildEnvironment(manifest *Manifest, requested []string, base []string) ([]string, error) {
	byID := make(map[string]Entry, len(manifest.Entries))
	credentialNames := make(map[string]struct{}, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		byID[entry.ID] = entry
		if entry.Kind() == KindEnv {
			credentialNames[entry.Ref()] = struct{}{}
		}
	}

	values := parseEnvironment(base)
	result := make([]string, 0, len(values)+len(requested)+2)
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := values[name]
		if !safeRuntimeVariable(name) {
			continue
		}
		if _, mapped := credentialNames[name]; mapped || credentialVariable(name) {
			continue
		}
		result = append(result, name+"="+value)
	}

	seenRequested := make(map[string]struct{}, len(requested))
	for _, id := range requested {
		if _, seen := seenRequested[id]; seen {
			continue
		}
		seenRequested[id] = struct{}{}
		entry, ok := byID[id]
		if !ok {
			return nil, errors.New("requested credential is not declared")
		}
		if entry.Kind() != KindEnv || entry.Ref() == "" {
			return nil, errors.New("requested credential cannot be injected into the agent environment")
		}
		value, present := values[entry.Ref()]
		if !present || value == "" {
			return nil, errors.New("requested credential is unavailable")
		}
		result = append(result, entry.Ref()+"="+value)
	}
	return result, nil
}

func parseEnvironment(values []string) map[string]string {
	parsed := make(map[string]string, len(values))
	for _, value := range values {
		name, contents, ok := strings.Cut(value, "=")
		if ok && name != "" {
			parsed[name] = contents
		}
	}
	return parsed
}

func safeRuntimeVariable(name string) bool {
	if strings.HasPrefix(name, "LC_") {
		return true
	}
	switch name {
	case "HOME", "USER", "LOGNAME", "PATH", "SHELL", "TERM", "COLORTERM", "LANG", "TMPDIR", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME":
		return true
	default:
		return false
	}
}

func credentialVariable(name string) bool {
	upper := strings.ToUpper(name)
	for _, marker := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "CREDENTIAL", "AUTH", "AWS_", "GITHUB_"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}
