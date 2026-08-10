package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConformanceOfficialFixtures evaluates Encode against the applicable
// official TOON encode fixtures vendored in testdata/conformance. The
// invariant is strict: the encoder either produces the exact expected TOON
// document or rejects the input as an unsupported form. It must never
// silently succeed with a different document.
func TestConformanceOfficialFixtures(t *testing.T) {
	files := []struct {
		name        string
		full        bool // true when every applicable case must conform
		minMatch    int  // floor on conformed cases so the supported set cannot silently shrink
		minRejected int  // floor on rejected cases so the rejection boundary cannot silently shrink
	}{
		{name: "primitives", full: true},
		{name: "objects", full: true},
		{name: "arrays-primitive", full: true},
		{name: "arrays-tabular", minMatch: 7},
		{name: "objects-keyed", minMatch: 4, minRejected: 1},
	}

	for _, fc := range files {
		t.Run(fc.name, func(t *testing.T) {
			path := filepath.Join("testdata", "conformance", fc.name+".json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture %s: %v", path, err)
			}
			var fixture struct {
				Version string `json:"version"`
				Tests   []struct {
					Name           string          `json:"name"`
					Input          json.RawMessage `json:"input"`
					Expected       string          `json:"expected"`
					Options        json.RawMessage `json:"options"`
					MinSpecVersion string          `json:"minSpecVersion"`
				} `json:"tests"`
			}
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatalf("parse fixture %s: %v", path, err)
			}
			if !strings.HasPrefix(fixture.Version, "4.") {
				t.Errorf("fixture %s baseline version %q is not a 4.x spec", path, fixture.Version)
			}

			matched, rejected := 0, 0
			for i, tc := range fixture.Tests {
				if skipped := conformanceSkip(t, i, tc.Options, tc.MinSpecVersion); skipped {
					continue
				}
				input, err := decodeJSONValue(tc.Input)
				if err != nil {
					t.Errorf("%s[%d] %s: decode input: %v", fc.name, i, tc.Name, err)
					continue
				}
				got, err := encodeValue(input)
				if err != nil {
					rejected++
					continue
				}
				if got != tc.Expected {
					t.Errorf("%s[%d] %s: encode mismatch\n got: %q\nwant: %q", fc.name, i, tc.Name, got, tc.Expected)
					continue
				}
				matched++
			}

			t.Logf("%s: %d conformed, %d rejected as unsupported", fc.name, matched, rejected)
			if fc.full {
				if rejected != 0 {
					t.Errorf("%s: expected full conformance but %d case(s) were rejected", fc.name, rejected)
				}
				if matched == 0 {
					t.Errorf("%s: no fixture cases conformed", fc.name)
				}
			}
			if matched < fc.minMatch {
				t.Errorf("%s: conformed count %d dropped below the floor %d", fc.name, matched, fc.minMatch)
			}
			if rejected < fc.minRejected {
				t.Errorf("%s: rejected count %d dropped below the floor %d", fc.name, rejected, fc.minRejected)
			}
		})
	}
}

// conformanceSkip reports whether a fixture test is outside the supported
// subset for reasons the fixture records (delimiter or indentation options,
// or a newer minimum spec version). Returns skipped=true.
func conformanceSkip(t *testing.T, index int, options json.RawMessage, minVersion string) bool {
	t.Helper()
	if len(options) > 0 && string(options) != "null" {
		var opts struct {
			Delimiter  *string `json:"delimiter"`
			IndentSize *int    `json:"indentSize"`
		}
		if err := json.Unmarshal(options, &opts); err != nil {
			t.Errorf("fixture[%d]: parse options: %v", index, err)
			return true
		}
		if opts.Delimiter != nil && *opts.Delimiter != "," {
			return true // non-comma delimiters are out of scope
		}
		if opts.IndentSize != nil && *opts.IndentSize != 2 {
			return true // non-default indentation is out of scope
		}
	}
	if minVersion != "" && minVersion > SpecVersion {
		return true
	}
	return false
}

// decodeJSONValue builds an ordered value from raw JSON, preserving object
// key order as encountered.
func decodeJSONValue(raw json.RawMessage) (*value, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	v, err := decodeJSONToken(dec)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func decodeJSONToken(dec *json.Decoder) (*value, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			v := &value{k: kindObject}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key := keyTok.(string)
				child, err := decodeJSONToken(dec)
				if err != nil {
					return nil, err
				}
				v.fields = append(v.fields, field{name: key, val: child})
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return v, nil
		case '[':
			v := &value{k: kindArray}
			for dec.More() {
				child, err := decodeJSONToken(dec)
				if err != nil {
					return nil, err
				}
				v.arr = append(v.arr, child)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return v, nil
		default:
			return nil, fmt.Errorf("unexpected delimiter %v", t)
		}
	case string:
		return &value{k: kindScalar, sk: scalarString, s: t}, nil
	case json.Number:
		return &value{k: kindScalar, sk: scalarNumber, s: canonicalNumber(t.String())}, nil
	case bool:
		if t {
			return &value{k: kindScalar, sk: scalarBool, s: "true"}, nil
		}
		return &value{k: kindScalar, sk: scalarBool, s: "false"}, nil
	case nil:
		return &value{k: kindScalar, sk: scalarNull}, nil
	default:
		return nil, fmt.Errorf("unexpected token %T", tok)
	}
}
