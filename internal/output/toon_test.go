package output

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

// TestWriteTabularArray covers the tabular array form used by list views
// (struct field order is preserved for deterministic output).
func TestWriteTabularArray(t *testing.T) {
	value := struct {
		Tasks []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"tasks"`
	}{
		Tasks: []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
		}{
			{ID: "one", Title: "First task", Status: "active"},
			{ID: "two", Title: "Second task", Status: "waiting"},
		},
	}

	var buf bytes.Buffer
	if err := Write(&buf, value); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	want := "tasks[2]{id,title,status}:\n  one,First task,active\n  two,Second task,waiting\n"
	if buf.String() != want {
		t.Fatalf("Write() = %q, want %q", buf.String(), want)
	}
}

func TestWriteHeterogeneousTabularArray(t *testing.T) {
	value := map[string]any{"tasks": []any{
		map[string]any{"id": "one", "status": "stopped"},
		map[string]any{"id": "two", "status": "waiting", "reason": "needs review"},
	}}

	got, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	want := "tasks[2]{id,status,reason}:\n  one,stopped,null\n  two,waiting,needs review"
	if got != want {
		t.Fatalf("Encode() = %q, want %q", got, want)
	}
}

// TestSchemaOutputs pins every output form the CLI emits today, including
// stable field behavior, quoting, and empty arrays.
func TestSchemaOutputs(t *testing.T) {
	type homeView struct {
		Bin         string   `json:"bin"`
		Description string   `json:"description"`
		Tasks       []string `json:"tasks"`
		Help        []string `json:"help"`
	}
	type worker struct {
		ID              string   `json:"id"`
		ProtocolVersion int      `json:"protocol_version"`
		Architecture    string   `json:"architecture"`
		OperatingSystem string   `json:"operating_system"`
		Features        []string `json:"features"`
	}
	type errorEnvelope struct {
		Error struct {
			Category  string `json:"category"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
			Recovery  string `json:"recovery"`
		} `json:"error"`
	}

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "home view with empty task list",
			value: homeView{
				Bin:         "~/bin/akagent",
				Description: "Manage local coding-agent tasks with durable state",
				Tasks:       []string{},
				Help: []string{
					"Run `akagent id generate` to create a task ID",
				},
			},
			want: "bin: ~/bin/akagent\ndescription: Manage local coding-agent tasks with durable state\ntasks: []\nhelp[1]: Run `akagent id generate` to create a task ID\n",
		},
		{
			name: "worker inspect",
			value: struct {
				Worker worker `json:"worker"`
			}{
				Worker: worker{
					ID:              "local",
					ProtocolVersion: 1,
					Architecture:    "arm64",
					OperatingSystem: "linux",
					Features:        []string{"tmux", "git-worktree"},
				},
			},
			want: "worker:\n  id: local\n  protocol_version: 1\n  architecture: arm64\n  operating_system: linux\n  features[2]: tmux,git-worktree\n",
		},
		{
			name: "structured error",
			value: errorEnvelope{Error: struct {
				Category  string `json:"category"`
				Message   string `json:"message"`
				Retryable bool   `json:"retryable"`
				Recovery  string `json:"recovery"`
			}{
				Category:  "usage",
				Message:   "Unknown command: [\"banana\"]",
				Retryable: false,
				Recovery:  "Run `akagent --help`",
			}},
			want: "error:\n  category: usage\n  message: \"Unknown command: [\\\"banana\\\"]\"\n  retryable: false\n  recovery: Run `akagent --help`\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Write(&buf, tc.value); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			if buf.String() != tc.want {
				t.Fatalf("Write() = %q, want %q", buf.String(), tc.want)
			}
		})
	}
}

// TestEncodingQuotesAndEmptyArrays verifies quoting rules and empty-array form
// on the boundary directly, mirroring the upstream contract.
func TestEncodingQuotesAndEmptyArrays(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"empty string", map[string]any{"v": ""}, `v: ""`},
		{"looks like boolean", map[string]any{"v": "true"}, `v: "true"`},
		{"looks like number", map[string]any{"v": "42"}, `v: "42"`},
		{"leading plus", map[string]any{"v": "+1"}, `v: "+1"`},
		{"leading hash", map[string]any{"v": "#a"}, `v: "#a"`},
		{"leading hyphen", map[string]any{"v": "- item"}, `v: "- item"`},
		{"colon", map[string]any{"v": "a:b"}, `v: "a:b"`},
		{"comma", map[string]any{"v": "a,b"}, `v: "a,b"`},
		{"newline escaped", map[string]any{"v": "l1\nl2"}, `v: "l1\nl2"`},
		{"control escaped", map[string]any{"v": "a\u0004b"}, `v: "a\u0004b"`},
		{"empty array", map[string]any{"items": []string{}}, "items: []"},
		{"empty root array", []string{}, "[]"},
		{"root array overflows to inline", []int{1, 2, 3}, "[3]: 1,2,3"},
		{"quoted key", map[string]any{"full name": "Ada"}, `"full name": Ada`},
		{"numeric negative zero", map[string]any{"v": -0.0}, "v: 0"},
		{"nan to null", map[string]any{"v": math.NaN()}, "v: null"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Encode(tc.value)
			if err != nil {
				t.Fatalf("Encode(%v) error = %v", tc.value, err)
			}
			if got != tc.want {
				t.Errorf("Encode(%v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestUnsupportedFormsGuarded ensures shapes outside the supported subset fail
// loudly instead of silently emitting a non-conforming document.
func TestUnsupportedFormsGuarded(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{"nested field group in tabular", map[string]any{"orders": []any{
			map[string]any{"id": 1, "customer": map[string]any{"name": "Ada"}},
			map[string]any{"id": 2, "customer": map[string]any{"name": "Bob"}},
		}}},
		{"mixed arrays", map[string]any{"items": []any{1, "text", map[string]any{"a": 1}}}},
		{"array of arrays", map[string]any{"grid": []any{[]any{1, 2}, []any{3, 4}}}},
		{"empty object element disqualifies tabular", map[string]any{"items": []any{map[string]any{}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Encode(tc.value); err == nil {
				t.Errorf("Encode(%v) succeeded, want an unsupported-form error", tc.value)
			} else if !errors.Is(err, ErrUnsupported) {
				t.Errorf("Encode(%v) error = %v, want ErrUnsupported", tc.value, err)
			}
		})
	}
}

// TestKeyedTabularRejected ensures objects that satisfy the spec's keyed
// tabular detection (§9.5) are rejected loudly rather than silently emitted as
// ordinary nested objects, and that non-eligible objects still nest normally.
func TestKeyedTabularRejected(t *testing.T) {
	eligible := []struct {
		name  string
		value any
	}{
		{
			"object field with uniform object values",
			map[string]any{"environments": map[string]any{
				"production": map[string]any{"region": "eu-central-1", "replicas": 6},
				"staging":    map[string]any{"region": "eu-central-1", "replicas": 2},
			}},
		},
		{
			"eligible root object",
			map[string]any{
				"production": map[string]any{"region": "eu-central-1", "replicas": 6},
				"staging":    map[string]any{"region": "eu-central-1", "replicas": 2},
			},
		},
		{
			"uniform nested object columns",
			map[string]any{"envs": map[string]any{
				"prod": map[string]any{"geo": map[string]any{"lat": 1.5, "lon": 2.5}},
				"dev":  map[string]any{"geo": map[string]any{"lat": 3, "lon": 4}},
			}},
		},
	}
	for _, tc := range eligible {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Encode(tc.value); err == nil {
				t.Errorf("Encode(%v) succeeded, want keyed-tabular rejection", tc.value)
			} else if !errors.Is(err, ErrUnsupported) {
				t.Errorf("Encode(%v) error = %v, want ErrUnsupported", tc.value, err)
			}
		})
	}

	// A single-entry object and a multi-entry object with a primitive value are
	// not keyed-eligible and must still encode as ordinary nested objects.
	notEligible := []struct {
		name  string
		value any
		want  string
	}{
		{
			"single-entry object nests",
			map[string]any{"worker": map[string]any{"id": "local", "arch": "arm64"}},
			"worker:\n  arch: arm64\n  id: local",
		},
		{
			"differing key sets nest",
			map[string]any{"envs": map[string]any{
				"prod": map[string]any{"region": "x"},
				"dev":  map[string]any{"zone": "y"},
			}},
			"envs:\n  dev:\n    zone: y\n  prod:\n    region: x",
		},
	}
	for _, tc := range notEligible {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Encode(tc.value)
			if err != nil {
				t.Fatalf("Encode(%v) error = %v", tc.value, err)
			}
			if got != tc.want {
				t.Errorf("Encode(%v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestDuplicateFieldsRejected ensures two effective JSON field names that
// collide (including a json tag colliding with another field) are rejected
// rather than producing duplicate TOON keys or tabular headers.
func TestDuplicateFieldsRejected(t *testing.T) {
	// Build a value with two fields sharing the same json tag via reflection
	// to avoid go vet warning on duplicate json tags in a composite literal.
	colliding := makeStructWithDupTag(t)

	// A json tag colliding with another field's effective name (Go field name).
	goNameCollision := struct {
		X string
		Y string `json:"X"`
	}{X: "x", Y: "y"}

	// json tag collides with a skipped field (json:"-") - only one effective.
	tagVsField := struct {
		Name string `json:"x"`
		X    string `json:"-"`
	}{Name: "n", X: "hidden"}

	for _, tc := range []struct {
		name    string
		value   any
		wantErr bool
	}{
		{"two tags with the same name", map[string]any{"v": colliding}, true},
		{"json tag collides with another effective name", map[string]any{"v": goNameCollision}, true},
		{"json tag collides with skipped field", map[string]any{"v": tagVsField}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Encode(tc.value)
			if tc.wantErr {
				if err == nil {
					t.Errorf("Encode(%v) succeeded, want duplicate-field rejection", tc.value)
				} else if !errors.Is(err, ErrUnsupported) {
					t.Errorf("Encode(%v) error = %v, want ErrUnsupported", tc.value, err)
				}
			} else {
				if err != nil {
					t.Errorf("Encode(%v) unexpected error = %v", tc.value, err)
				}
			}
		})
	}
}

// makeStructWithDupTag builds a struct value with two fields that share the
// same json tag, using reflection to avoid go vet flagging the literal.
func makeStructWithDupTag(t *testing.T) any {
	t.Helper()
	st := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(""), Tag: `json:"name"`},
		{Name: "B", Type: reflect.TypeOf(""), Tag: `json:"name"`},
	})
	val := reflect.New(st).Elem()
	val.Field(0).SetString("a")
	val.Field(1).SetString("b")
	return val.Interface()
}

func TestWriteError(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteError(&buf, "conflict", "Task inputs conflict", false, "akagent task inspect <id>"); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	for _, want := range []string{"error:", "category: conflict", "message: Task inputs conflict", "retryable: false", "recovery: akagent task inspect <id>"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("WriteError() = %q, want to contain %q", buf.String(), want)
		}
	}
}
