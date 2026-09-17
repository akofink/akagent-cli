package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// EncodeJSON converts value to compact JSON using the same typed views as the
// default TOON protocol. HTML characters are left unescaped so titles and
// recovery text remain literal interchange data. The returned string has no
// trailing newline.
func EncodeJSON(value any) (string, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", fmt.Errorf("encode JSON: %w", err)
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// WriteJSON encodes value as compact JSON followed by a newline.
func WriteJSON(writer io.Writer, value any) error {
	encoded, err := EncodeJSON(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(writer, encoded); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}
