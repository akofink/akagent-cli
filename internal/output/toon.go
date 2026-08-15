// Package output encodes akagent's protocol output as TOON.
//
// The supported contract is a constrained subset of the TOON specification
// (toon-format/spec) version 4.1. See docs/toon.md for the pinned version,
// the supported forms, and the documented deviations. The encoder is
// self-contained so that every emitted value is validated against the pinned
// contract rather than inheriting the behavior of an external encoder.
package output

import (
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// SpecVersion is the TOON specification (toon-format/spec) version this
	// encoder targets.
	SpecVersion = "4.1"
	indentUnit  = "  "
	delimiter   = ','
)

// ErrUnsupported is returned when a value uses a TOON form outside the
// supported subset (see docs/toon.md). The encoder rejects such values loudly
// instead of emitting output that could violate the pinned contract.
var ErrUnsupported = errors.New("unsupported TOON form")

// kind distinguishes the node shapes produced by normalization.
type kind uint8

const (
	kindScalar kind = iota
	kindObject
	kindArray
)

// scalarKind distinguishes scalar value types for rendering.
type scalarKind uint8

const (
	scalarString scalarKind = iota
	scalarBool
	scalarNumber
	scalarNull
)

type field struct {
	name string
	val  *value
}

type value struct {
	k      kind
	fields []field
	order  []string
	arr    []*value
	sk     scalarKind
	s      string // raw string (scalarString) or canonical token (bool/number)
}

// Write encodes value as TOON to writer, followed by a newline.
func Write(writer io.Writer, value any) error {
	encoded, err := Encode(value)
	if err != nil {
		return fmt.Errorf("encode TOON: %w", err)
	}
	if _, err := fmt.Fprintln(writer, encoded); err != nil {
		return fmt.Errorf("write TOON: %w", err)
	}
	return nil
}

// Encode converts value to a TOON string conforming to the supported subset
// of the pinned specification. The returned string has no trailing newline.
func Encode(value any) (string, error) {
	root, err := normalize(value)
	if err != nil {
		return "", err
	}
	return encodeValue(root)
}

func encodeValue(root *value) (string, error) {
	var b strings.Builder
	if err := render(&b, root, 0, true); err != nil {
		return "", err
	}
	// Encoding never adds a terminal newline (spec §12); the CLI's Write adds
	// the line terminator for stdout.
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// WriteError encodes an error envelope to writer. See docs/protocol.md.
func WriteError(writer io.Writer, category, message string, retryable bool, recovery string) error {
	return Write(writer, errorEnvelope{
		Error: protocolError{
			Category:  category,
			Message:   message,
			Retryable: retryable,
			Recovery:  recovery,
		},
	})
}

type errorEnvelope struct {
	Error protocolError `json:"error"`
}

type protocolError struct {
	Category  string `json:"category"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Recovery  string `json:"recovery"`
}

// normalize converts an arbitrary Go value into the ordered internal
// representation. Structs preserve field declaration order; map keys are
// sorted for determinism (a documented deviation, see docs/toon.md).
func normalize(v any) (*value, error) {
	if v == nil {
		return &value{k: kindScalar, sk: scalarNull}, nil
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return &value{k: kindScalar, sk: scalarNull}, nil
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.String:
		return scalarStringValue(rv.String())
	case reflect.Bool:
		return &value{k: kindScalar, sk: scalarBool, s: strconv.FormatBool(rv.Bool())}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &value{k: kindScalar, sk: scalarNumber, s: strconv.FormatInt(rv.Int(), 10)}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return &value{k: kindScalar, sk: scalarNumber, s: strconv.FormatUint(rv.Uint(), 10)}, nil
	case reflect.Float32, reflect.Float64:
		return &value{k: kindScalar, sk: scalarNumber, s: formatFloat(rv.Float())}, nil
	case reflect.Struct:
		if rv.Type() == reflect.TypeOf(time.Time{}) {
			return scalarStringValue(rv.Interface().(time.Time).Format(time.RFC3339Nano))
		}
		return normalizeStruct(rv)
	case reflect.Map:
		return normalizeMap(rv)
	case reflect.Slice, reflect.Array:
		return normalizeArray(rv)
	default:
		return nil, fmt.Errorf("%w: cannot encode %s", ErrUnsupported, rv.Kind())
	}
}

func scalarStringValue(s string) (*value, error) {
	if !utf8.ValidString(s) {
		return nil, fmt.Errorf("cannot encode invalid UTF-8 string")
	}
	return &value{k: kindScalar, sk: scalarString, s: s}, nil
}

func normalizeStruct(rv reflect.Value) (*value, error) {
	out := &value{k: kindObject}
	t := rv.Type()
	seen := make(map[string]bool, t.NumField())
	for i := 0; i < rv.NumField(); i++ {
		ft := t.Field(i)
		if ft.PkgPath != "" {
			continue // unexported
		}
		tag := ft.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := ft.Name
		omitEmpty := false
		if tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] != "" {
				name = parts[0]
			}
			for _, opt := range parts[1:] {
				if opt == "omitempty" {
					omitEmpty = true
				}
			}
		}
		out.order = append(out.order, name)
		fv := rv.Field(i)
		if omitEmpty && fv.IsZero() {
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("%w: duplicate JSON field name %q", ErrUnsupported, name)
		}
		seen[name] = true
		child, err := normalize(fv.Interface())
		if err != nil {
			return nil, err
		}
		out.fields = append(out.fields, field{name: name, val: child})
	}
	return out, nil
}

func normalizeMap(rv reflect.Value) (*value, error) {
	out := &value{k: kindObject}
	keys := rv.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	for _, k := range keys {
		child, err := normalize(rv.MapIndex(k).Interface())
		if err != nil {
			return nil, err
		}
		out.fields = append(out.fields, field{name: fmt.Sprint(k.Interface()), val: child})
	}
	return out, nil
}

func normalizeArray(rv reflect.Value) (*value, error) {
	out := &value{k: kindArray}
	for i := 0; i < rv.Len(); i++ {
		child, err := normalize(rv.Index(i).Interface())
		if err != nil {
			return nil, err
		}
		out.arr = append(out.arr, child)
	}
	return out, nil
}

// render writes the root value, which may be a primitive, object, or array.
func render(b *strings.Builder, v *value, depth int, root bool) error {
	switch v.k {
	case kindScalar:
		tok, err := scalarToken(v)
		if err != nil {
			return err
		}
		b.WriteString(tok)
		return nil
	case kindObject:
		if root && len(v.fields) == 0 {
			return nil // empty root object is an empty document
		}
		if keyedTabularEligible(v) {
			return fmt.Errorf("%w: keyed tabular objects are not supported", ErrUnsupported)
		}
		return renderObject(b, v, depth)
	case kindArray:
		return renderRootArray(b, v)
	default:
		return fmt.Errorf("%w: unknown shape", ErrUnsupported)
	}
}

func renderObject(b *strings.Builder, v *value, depth int) error {
	for _, f := range v.fields {
		if err := renderField(b, f, depth); err != nil {
			return err
		}
	}
	return nil
}

func renderField(b *strings.Builder, f field, depth int) error {
	indent := strings.Repeat(indentUnit, depth)
	key := quoteKey(f.name)
	switch f.val.k {
	case kindScalar:
		tok, err := scalarToken(f.val)
		if err != nil {
			return err
		}
		b.WriteString(indent + key + ": " + tok + "\n")
	case kindObject:
		if keyedTabularEligible(f.val) {
			return fmt.Errorf("%w: keyed tabular objects are not supported", ErrUnsupported)
		}
		b.WriteString(indent + key + ":\n")
		return renderObject(b, f.val, depth+1)
	case kindArray:
		if len(f.val.arr) == 0 {
			b.WriteString(indent + key + ": []\n")
			return nil
		}
		return renderArrayLines(b, indent, key, true, f.val, depth+1)
	default:
		return fmt.Errorf("%w: unknown shape", ErrUnsupported)
	}
	return nil
}

func renderRootArray(b *strings.Builder, v *value) error {
	if len(v.arr) == 0 {
		b.WriteString("[]")
		return nil
	}
	return renderArrayLines(b, "", "", false, v, 1)
}

// renderArrayLines emits a non-empty array. Root arrays pass hasKey=false and
// an empty key; object-field arrays pass the already-encoded key with
// hasKey=true.
func renderArrayLines(b *strings.Builder, indent, key string, hasKey bool, v *value, rowDepth int) error {
	// Primitive (scalar) array: inline form.
	if allScalar(v.arr) {
		tokens := make([]string, 0, len(v.arr))
		for _, e := range v.arr {
			tok, err := scalarToken(e)
			if err != nil {
				return err
			}
			tokens = append(tokens, tok)
		}
		line := fmt.Sprintf("[%d]: %s", len(tokens), strings.Join(tokens, string(delimiter)))
		if hasKey {
			line = key + line
		}
		b.WriteString(indent + line + "\n")
		return nil
	}

	// Tabular array of uniform scalar objects.
	return renderTabularArray(b, indent, key, hasKey, v, rowDepth)
}

func allScalar(arr []*value) bool {
	for _, e := range arr {
		if e.k != kindScalar {
			return false
		}
	}
	return true
}

func renderTabularArray(b *strings.Builder, indent, key string, hasKey bool, v *value, rowDepth int) error {
	// Every element must be a non-empty object with scalar leaves. Missing
	// fields are rendered as null so optional fields can vary between rows
	// while the tabular schema remains deterministic.
	names, err := tabularFields(v.arr)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("[%d]{", len(v.arr))
	for i, n := range names {
		if i > 0 {
			header += string(delimiter)
		}
		header += quoteKey(n)
	}
	header += "}:\n"
	if hasKey {
		header = key + header
	}
	b.WriteString(indent + header)
	for _, e := range v.arr {
		row := make([]string, 0, len(names))
		for _, n := range names {
			tok, err := scalarToken(fieldValue(e, n))
			if err != nil {
				return err
			}
			row = append(row, tok)
		}
		b.WriteString(strings.Repeat(indentUnit, rowDepth) + strings.Join(row, string(delimiter)) + "\n")
	}
	return nil
}

func fieldValue(obj *value, name string) *value {
	for i := range obj.fields {
		if obj.fields[i].name == name {
			return obj.fields[i].val
		}
	}
	return &value{k: kindScalar, sk: scalarNull}
}

// tabularFields returns the ordered union of field names if the array is
// composed of non-empty objects with scalar fields, or an error for any other
// shape. Struct-based rows use their declaration order, while map-based rows
// use first appearance.
func tabularFields(arr []*value) ([]string, error) {
	if len(arr) == 0 {
		return nil, fmt.Errorf("%w: empty tabular array", ErrUnsupported)
	}
	names := make([]string, 0)
	seenNames := make(map[string]bool)
	for i, e := range arr {
		if e.k != kindObject {
			return nil, fmt.Errorf("%w: tabular row %d is not an object", ErrUnsupported, i)
		}
		if len(e.fields) == 0 {
			return nil, fmt.Errorf("%w: tabular row %d is an empty object", ErrUnsupported, i)
		}
		seen := make(map[string]bool, len(e.fields))
		for _, f := range e.fields {
			if f.val.k != kindScalar {
				return nil, fmt.Errorf("%w: tabular field %q is not a primitive at row %d", ErrUnsupported, f.name, i)
			}
			if seen[f.name] {
				return nil, fmt.Errorf("%w: duplicate tabular field %q at row %d", ErrUnsupported, f.name, i)
			}
			seen[f.name] = true
			if !seenNames[f.name] {
				names = append(names, f.name)
				seenNames[f.name] = true
			}
		}
	}
	if len(arr) > 0 && len(arr[0].order) > 0 {
		present := make(map[string]bool, len(names))
		for _, name := range names {
			present[name] = true
		}
		ordered := make([]string, 0, len(names))
		seen := make(map[string]bool, len(names))
		for _, name := range arr[0].order {
			if present[name] && !seen[name] {
				ordered = append(ordered, name)
				seen[name] = true
			}
		}
		for _, name := range names {
			if !seen[name] {
				ordered = append(ordered, name)
			}
		}
		names = ordered
	}
	return names, nil
}

// keyedTabularEligible reports whether an object, in object-field or root
// position, satisfies the spec's keyed-tabular detection (§9.5). Because the
// supported subset does not emit keyed tabular form, such objects are rejected
// rather than silently encoded as ordinary nested objects.
func keyedTabularEligible(v *value) bool {
	if len(v.fields) < 2 {
		return false
	}
	for _, f := range v.fields {
		if f.val.k != kindObject || len(f.val.fields) == 0 {
			return false
		}
	}
	names := objectNameSet(v.fields[0].val)
	for _, f := range v.fields[1:] {
		if !sameNameSet(names, f.val) {
			return false
		}
	}
	for _, n := range names {
		if !columnUniform(v.fields, n) {
			return false
		}
	}
	return true
}

func objectNameSet(obj *value) []string {
	names := make([]string, 0, len(obj.fields))
	for _, f := range obj.fields {
		names = append(names, f.name)
	}
	sort.Strings(names)
	return names
}

func sameNameSet(a []string, obj *value) bool {
	b := objectNameSet(obj)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// columnUniform reports whether the column at key across all entry objects is
// uniform-primitive or nested-uniform, per §9.3's column rules.
func columnUniform(entries []field, key string) bool {
	vals := make([]*value, 0, len(entries))
	for _, e := range entries {
		vals = append(vals, fieldValue(e.val, key))
	}
	return valuesUniform(vals)
}

func valuesUniform(vals []*value) bool {
	allScalar := true
	for _, v := range vals {
		if v.k != kindScalar {
			allScalar = false
			break
		}
	}
	if allScalar {
		return true
	}
	for _, v := range vals {
		if v.k != kindObject || len(v.fields) == 0 {
			return false
		}
	}
	names := objectNameSet(vals[0])
	for _, v := range vals[1:] {
		if !sameNameSet(names, v) {
			return false
		}
	}
	for _, n := range names {
		cols := make([]*value, 0, len(vals))
		for _, v := range vals {
			cols = append(cols, fieldValue(v, n))
		}
		if !valuesUniform(cols) {
			return false
		}
	}
	return true
}

func scalarToken(v *value) (string, error) {
	switch v.sk {
	case scalarNull:
		return "null", nil
	case scalarBool, scalarNumber:
		return v.s, nil
	case scalarString:
		if needsQuote(v.s) {
			return `"` + escapeString(v.s) + `"`, nil
		}
		return v.s, nil
	default:
		return "", fmt.Errorf("%w: unknown scalar", ErrUnsupported)
	}
}

// formatFloat renders f in the spec's canonical decimal form, using exponent
// notation only outside the canonical range. NaN and infinities become null.
func formatFloat(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "null"
	}
	if f == 0 {
		return "0" // also normalizes -0
	}
	abs := math.Abs(f)
	if abs >= 1e-6 && abs < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strconv.FormatFloat(f, 'e', -1, 64)
}

// canonicalNumber normalizes a JSON number token to the spec's canonical
// decimal form. It preserves large integers exactly rather than rounding them
// through float64.
func canonicalNumber(token string) string {
	neg := strings.HasPrefix(token, "-")
	t := strings.TrimPrefix(token, "-")
	if strings.IndexAny(t, ".eE") < 0 {
		t = strings.TrimLeft(t, "0")
		if t == "" {
			t = "0"
		}
		if neg && t != "0" {
			return "-" + t
		}
		return t
	}
	f, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return "0"
	}
	return formatFloat(f)
}

var unquotedKeyRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
var numericLikeRE = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?(?:e[+-]?[0-9]+)?$`)

// quoteKey encodes an object key per spec §7.3.
func quoteKey(s string) string {
	if unquotedKeyRE.MatchString(s) {
		return s
	}
	return `"` + escapeString(s) + `"`
}

// needsQuote reports whether a string value must be quoted per spec §7.2.
func needsQuote(s string) bool {
	if s == "" {
		return true
	}
	if s == "true" || s == "false" || s == "null" {
		return true
	}
	if numericLikeRE.MatchString(s) {
		return true
	}
	if s == "-" || strings.HasPrefix(s, "-") {
		return true
	}
	if strings.HasPrefix(s, "#") {
		return true
	}
	if s[0] == ' ' || s[0] == '\t' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t' {
		return true
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ':' || c == '"' || c == '\\' || c == delimiter ||
			c == '[' || c == ']' || c == '{' || c == '}' || c < 0x20 {
			return true
		}
	}
	return false
}

// escapeString escapes a string for placement between quotes per spec §7.1.
func escapeString(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				b.WriteString(fmt.Sprintf(`\u%04x`, c))
			} else {
				b.WriteByte(c)
			}
		}
	}
	return b.String()
}
