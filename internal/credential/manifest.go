package credential

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// ConfigEnv overrides the default manifest path. It mirrors the
// AKAGENT_SOURCE_DIR convention for non-default local configuration.
const ConfigEnv = "AKAGENT_CREDENTIALS"

const (
	// MaxSupportedVersion is the highest manifest schema version this
	// implementation understands. Newer versions are rejected explicitly.
	MaxSupportedVersion = 1
)

// manifestFieldOrder is the exact, required column order of the manifest so row
// parsing is unambiguous.
var manifestFieldOrder = []string{"id", "type", "source", "required_for"}

// DefaultConfigPath returns ~/.config/akagent/credentials.toon.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home + "/.config/akagent/credentials.toon"
}

// ConfigPath returns the AKAGENT_CREDENTIALS override when set, otherwise the
// default manifest path.
func ConfigPath() string {
	if p := os.Getenv(ConfigEnv); p != "" {
		return p
	}
	return DefaultConfigPath()
}

// Load reads and parses the manifest at path. A missing manifest is a
// definitive empty state, not an error.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{Version: 1}, nil
		}
		return nil, err
	}
	return Parse(data)
}

// Parse parses the tabular TOON manifest:
//
//	version: 1
//	credentials[N]{id,type,source,required_for}:
//	  git-ssh,ssh_key,file:/path/git_ed25519,git
//	  gh-token,api_token,env:GITHUB_TOKEN,
//
// A trailing empty required_for marks an optional credential. Malformed input
// returns a descriptive error so the CLI can surface a structured failure.
// Parsing uses TOON-aware field splitting to handle quoted fields, escapes,
// and embedded commas. Raw manifest content is never included in error output.
func Parse(data []byte) (*Manifest, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	m := &Manifest{Version: 1}

	var headerFields []string
	declaredLen := -1
	providedLen := 0
	inCredentials := false
	seenIDs := make(map[string]struct{})

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if !inCredentials {
			if strings.HasPrefix(line, "version:") {
				value := strings.TrimSpace(strings.TrimPrefix(line, "version:"))
				version, err := strconv.Atoi(value)
				if err != nil || version < 1 {
					return nil, fmt.Errorf("invalid manifest version")
				}
				if version > MaxSupportedVersion {
					return nil, fmt.Errorf("unsupported manifest version %d", version)
				}
				m.Version = version
				continue
			}
			if strings.HasPrefix(line, "credentials") {
				fields, length, err := parseHeader(line)
				if err != nil {
					return nil, err
				}
				headerFields = fields
				declaredLen = length
				inCredentials = true
				continue
			}
			return nil, fmt.Errorf("unexpected manifest line")
		}

		if strings.HasPrefix(line, "credentials") || strings.HasPrefix(line, "version:") {
			return nil, fmt.Errorf("unexpected manifest line")
		}

		values, err := splitTOONFields(line)
		if err != nil {
			return nil, fmt.Errorf("malformed credential row")
		}
		if len(values) != len(headerFields) {
			return nil, fmt.Errorf("credential row field count mismatch")
		}

		var entry Entry
		for i, field := range headerFields {
			value := values[i]
			switch field {
			case "id":
				entry.ID = value
			case "type":
				entry.Type = value
			case "source":
				entry.Source = value
			case "required_for":
				entry.RequiredFor = value
			}
		}
		if entry.ID == "" {
			return nil, fmt.Errorf("credential row missing id")
		}
		if _, dup := seenIDs[entry.ID]; dup {
			return nil, fmt.Errorf("duplicate credential id")
		}
		seenIDs[entry.ID] = struct{}{}
		if entry.Source == "" {
			return nil, fmt.Errorf("credential missing source")
		}
		if kind := entry.Kind(); kind != KindFile && kind != KindEnv {
			return nil, fmt.Errorf("credential uses unsupported source kind")
		}
		if entry.Ref() == "" {
			return nil, fmt.Errorf("credential has empty source reference")
		}
		m.Entries = append(m.Entries, entry)
		providedLen++
	}

	if declaredLen >= 0 && declaredLen != providedLen {
		return nil, fmt.Errorf("credentials header declares %d entries but found %d", declaredLen, providedLen)
	}
	return m, nil
}

// splitTOONFields splits a TOON tabular row into fields, respecting quoted
// strings, escapes, and embedded commas. It follows the TOON string literal
// rules: double-quoted strings support backslash escapes, while delimiters
// and structural brackets are not accepted in unquoted fields.
func splitTOONFields(line string) ([]string, error) {
	var fields []string
	var current strings.Builder
	inQuote := false
	fieldQuoted := false
	quoteClosed := false
	escaped := false

	appendField := func() {
		value := current.String()
		if !fieldQuoted {
			value = strings.TrimSpace(value)
		}
		fields = append(fields, value)
		current.Reset()
		fieldQuoted = false
		quoteClosed = false
	}

	for _, r := range line {
		if escaped {
			if !inQuote {
				return nil, fmt.Errorf("escape outside quoted string")
			}
			switch r {
			case '"', '\\':
				current.WriteRune(r)
			case 'n':
				current.WriteRune('\n')
			case 'r':
				current.WriteRune('\r')
			case 't':
				current.WriteRune('\t')
			default:
				return nil, fmt.Errorf("invalid escape sequence")
			}
			escaped = false
			continue
		}

		if r == '\\' && inQuote {
			escaped = true
			continue
		}

		if r == '"' {
			if inQuote {
				inQuote = false
				quoteClosed = true
			} else {
				if quoteClosed || strings.TrimSpace(current.String()) != "" {
					return nil, fmt.Errorf("quoted string must start a field")
				}
				current.Reset()
				inQuote = true
				fieldQuoted = true
			}
			continue
		}

		if quoteClosed {
			if r == ',' {
				appendField()
				continue
			}
			if unicode.IsSpace(r) {
				continue
			}
			return nil, fmt.Errorf("quoted field has trailing content")
		}

		if r == ',' && !inQuote {
			appendField()
			continue
		}

		if !inQuote && (r == '{' || r == '}' || r == '[' || r == ']') {
			return nil, fmt.Errorf("invalid character in unquoted field")
		}

		current.WriteRune(r)
	}

	if inQuote {
		return nil, fmt.Errorf("unterminated quoted string")
	}
	if escaped {
		return nil, fmt.Errorf("trailing escape")
	}

	appendField()
	return fields, nil
}

// parseHeader parses "credentials[4]{id,type,source,required_for}:" (the count
// markers, and the colon, are optional) and verifies the exact field set.
func parseHeader(line string) ([]string, int, error) {
	open := strings.Index(line, "{")
	closeIndex := strings.Index(line, "}")
	if open < 0 || closeIndex < 0 || closeIndex <= open {
		return nil, 0, fmt.Errorf("malformed credentials header")
	}

	rawFields := strings.Split(line[open+1:closeIndex], ",")
	fields := make([]string, 0, len(rawFields))
	for _, f := range rawFields {
		fields = append(fields, strings.TrimSpace(f))
	}
	if len(fields) != len(manifestFieldOrder) {
		return nil, 0, fmt.Errorf("credentials header field count mismatch")
	}
	for i := range fields {
		if fields[i] != manifestFieldOrder[i] {
			return nil, 0, fmt.Errorf("unsupported credentials header fields")
		}
	}

	length := -1
	tail := strings.TrimSuffix(strings.TrimSpace(line[:open]), ":")
	left := strings.Index(tail, "credentials") + len("credentials")
	marker := strings.TrimSpace(tail[left:])
	if marker != "" {
		if !strings.HasPrefix(marker, "[") || !strings.HasSuffix(marker, "]") {
			return nil, 0, fmt.Errorf("malformed credentials header")
		}
		n, err := strconv.Atoi(marker[1 : len(marker)-1])
		if err != nil || n < 0 {
			return nil, 0, fmt.Errorf("invalid credentials count in header")
		}
		length = n
	}
	return fields, length, nil
}
