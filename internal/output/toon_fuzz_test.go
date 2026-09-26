package output

import (
	"math"
	"testing"
)

func FuzzEncode(f *testing.F) {
	// These values mirror the supported conformance and schema edge cases.
	f.Add("plain text", "name", int64(1), true)
	f.Add("", "full name", int64(-1), false)
	f.Add("true", "items", int64(0), true)
	f.Add("42", "a:b", int64(math.MaxInt64), false)
	f.Add("line1\nline2", "quote\"slash\\", int64(math.MinInt64), true)
	f.Add("# comment", "comma,key", int64(7), false)
	f.Add("\x00\t\r", "", int64(9), true)
	f.Add("\u2028λ🙂", "unicode.key", int64(-42), true)

	f.Fuzz(func(t *testing.T, text, key string, number int64, flag bool) {
		value := map[string]any{
			key: map[string]any{
				"text":   text,
				"number": number,
				"flag":   flag,
			},
			"items": []any{text, nil, number},
		}
		got, err := Encode(value)
		if err != nil {
			return
		}
		if got == "" || got[len(got)-1] == '\n' {
			t.Fatalf("Encode() returned invalid document boundary: %q", got)
		}
		again, err := Encode(value)
		if err != nil {
			t.Fatalf("second Encode() error = %v", err)
		}
		if again != got {
			t.Fatalf("Encode() is not deterministic: %q != %q", got, again)
		}
	})
}
