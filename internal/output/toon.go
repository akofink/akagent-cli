package output

import (
	"fmt"
	"io"

	"github.com/alpkeskin/gotoon"
)

type errorEnvelope struct {
	Error protocolError `json:"error"`
}

type protocolError struct {
	Category  string `json:"category"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Recovery  string `json:"recovery"`
}

func Write(writer io.Writer, value any) error {
	encoded, err := gotoon.Encode(value)
	if err != nil {
		return fmt.Errorf("encode TOON: %w", err)
	}
	if _, err := fmt.Fprintln(writer, encoded); err != nil {
		return fmt.Errorf("write TOON: %w", err)
	}
	return nil
}

func WriteError(writer io.Writer, category, message string, retryable bool, recovery string) error {
	return Write(writer, errorEnvelope{Error: protocolError{
		Category:  category,
		Message:   message,
		Retryable: retryable,
		Recovery:  recovery,
	}})
}
