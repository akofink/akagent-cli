package output

import (
	"bytes"
	"strings"
	"testing"
)

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

	var output bytes.Buffer
	if err := Write(&output, value); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	for _, expected := range []string{"tasks[2]{id,status,title}:", "one,active,First task", "two,waiting,Second task"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("Write() output = %q, want to contain %q", output.String(), expected)
		}
	}
}
