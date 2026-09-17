package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncodeJSONIsCompactAndLiteral(t *testing.T) {
	value := struct {
		Title    string            `json:"title"`
		Metadata map[string]string `json:"metadata,omitempty"`
		Total    int               `json:"total"`
	}{
		Title:    `JSON <output> & "test"`,
		Metadata: map[string]string{"zeta": "1", "alpha": "2"},
		Total:    1,
	}

	got, err := EncodeJSON(value)
	if err != nil {
		t.Fatalf("EncodeJSON() error = %v", err)
	}
	want := `{"title":"JSON <output> & \"test\"","metadata":{"alpha":"2","zeta":"1"},"total":1}`
	if got != want {
		t.Fatalf("EncodeJSON() = %q, want %q", got, want)
	}
	if strings.Contains(got, `\u003c`) || strings.Contains(got, `\u003e`) || strings.Contains(got, `\u0026`) {
		t.Fatalf("EncodeJSON() HTML-escaped interchange text: %q", got)
	}
}

func TestWriteJSONAppendsNewlineAndIsDeterministic(t *testing.T) {
	value := struct {
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
		Total int `json:"total"`
	}{
		Tasks: []struct {
			ID string `json:"id"`
		}{{ID: "one"}, {ID: "two"}},
		Total: 2,
	}

	var first, second bytes.Buffer
	if err := WriteJSON(&first, value); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	if err := WriteJSON(&second, value); err != nil {
		t.Fatalf("second WriteJSON() error = %v", err)
	}
	want := "{\"tasks\":[{\"id\":\"one\"},{\"id\":\"two\"}],\"total\":2}\n"
	if first.String() != want {
		t.Fatalf("WriteJSON() = %q, want %q", first.String(), want)
	}
	if first.String() != second.String() {
		t.Fatalf("WriteJSON() was not deterministic: %q vs %q", first.String(), second.String())
	}
}

func TestEncodeJSONRejectsUnsupportedValues(t *testing.T) {
	if _, err := EncodeJSON(make(chan int)); err == nil {
		t.Fatal("EncodeJSON() succeeded for a channel")
	}
}
