# TOON conformance fixtures

These JSON files are the official, language-agnostic TOON **encode** fixtures
published by the [TOON specification repository](https://github.com/toon-format/spec).

- Source: `toon-format/spec` `tests/fixtures/encode/`
- Spec revision (SHA): `62f16b369408180f1faf1cba7da1b46d1f336f12`
- Baseline spec version claimed by each file: `4.0`
- Pinned supported specification version: `4.1` (see `docs/toon.md`)

Only the fixtures applicable to the supported output subset are vendored here:

| File | Covers |
| --- | --- |
| `primitives.json` | Primitive scalar encoding: quoting, escaping, numbers, booleans, null |
| `objects.json` | Simple and nested objects, key encoding |
| `arrays-primitive.json` | Inline primitive arrays, empty arrays |
| `arrays-tabular.json` | Tabular arrays of uniform scalar objects |

`conformance_test.go` loads every fixture and asserts the encoder either
produces the exact expected TOON or rejects the value as an unsupported form.
No test may silently succeed with a different document.
