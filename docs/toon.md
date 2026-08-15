# TOON output contract

This document pins the TOON output contract that guards every agent-facing
CLI response.
It records the supported specification version, the encoder decision made for
this issue, the supported output subset, known deviations, and the token
measurement methodology and results.

## Pinned specification version

akagent targets **TOON specification version 4.1**, published by the
[`toon-format/spec`](https://github.com/toon-format/spec) repository.

Conformance fixtures are vendored from that repository at
`internal/output/testdata/conformance/`.
The `README.md` there records the exact upstream revision the fixtures were
copied from.
`internal/output/conformance_test.go` loads every vendored fixture and asserts
the encoder either produces the exact expected TOON document or rejects the
value as an unsupported form.
An input the encoder accepts must never be encoded differently from the
official expectation, and a value it cannot represent is rejected loudly with
an error rather than silently emitting a non-conforming document.

Run the conformance suite with:

```bash
go test ./internal/output -run TestConformanceOfficialFixtures -v
```

## Encoder decision

The previous encoder (`github.com/alpkeskin/gotoon` v0.1.1) was evaluated
against the pinned specification and found non-conforming:

- Empty arrays were emitted as `field[0]:` instead of the required `field: []`
  (section 9.1).
- Strings equal to or starting with `#` (the v4 comment marker, section 5.1)
  and numeric-like strings such as `+1` were left unquoted, so output could be
  misread by a conforming decoder.
- Object key order was re-sorted alphabetically rather than preserved in
  encounter order (section 2).

Because the CLI protocol is the stable product boundary and later commands
build on this contract, the encoder was **replaced** with a small,
self-contained encoder in `internal/output` that emits only the supported
subset and is validated against the official fixtures.
No external TOON encoder dependency remains.

## Supported output subset

The encoder emits exactly the forms the CLI and its documented schemas need:

| Form | Example |
| --- | --- |
| Primitive scalar field | `id: 019fe968-cebf-7d21-ade7-946d1d6c979c` |
| Boolean and numeric fields | `retryable: false`, `protocol_version: 1` |
| Nested object | `worker:` followed by indented fields |
| Non-empty primitive array (inline) | `tags[3]: admin,ops,dev` |
| Empty array | `tasks: []` |
| Tabular array of scalar objects | `tasks[2]{id,title,status}:` with one row per line and `null` for missing fields |
| Structured error envelope | `error:` with `category`, `message`, `retryable`, `recovery` |

Field order is the struct field declaration order.
This makes output deterministic and preserves the spec's encounter-order rule
for the structs every schema uses.
Map values are sorted by key, which is deterministic and documented as a
deviation (see below).

## Known deviations

The pinned contract targets spec 4.1 but deliberately scopes to the forms
above.
Anything outside that subset is rejected with an error, never silently encoded
incorrectly.
Documented deviations and boundaries:

- Unsupported delimiter and indentation options (non-comma delimiter, indent
  other than 2): the encoder always uses comma and 2-space indentation.
- Keyed tabular form (section 9.5) is not emitted; objects of objects remain
  ordinary nested objects.
- Nested field groups in tabular arrays (section 9.3) are not emitted; a
  tabular array whose columns are not all primitive scalars is rejected.
- List form for arrays containing non-object or nested values (section 9.4) is
  not emitted; such arrays are rejected.
- Map values sort their keys alphabetically instead of preserving encounter
  order (section 2), because Go maps are unordered.
  Struct-based schemas, which are the norm, preserve declaration order.
- Only the comma delimiter is supported, matching all current schemas.

The encoder normalizes `NaN` and infinities to `null` and returns an error for
strings that are not valid UTF-8, per section 3.
No comment lines are ever emitted, per section 5.1.

## Token measurement

Tokens are estimated with the deterministic proxy `ceil(len / 4)`, a common
heuristic approximation of the Claude and GPT tokenizers for English-heavy
text.
This is a reproducible proxy, not an exact tokenizer count, and the results
below are specific to these sample schemas.
They make no universal claim about TOON-vs-JSON savings.

Reproduce the measurement with:

```bash
go test ./internal/output -run TestTokenMeasurement -v
```

Recorded results (TOON tokens vs compact JSON tokens, lower is better):

| Sample | JSON | TOON |
| --- | ---: | ---: |
| home view | 74 | 70 |
| worker inspect | 33 | 31 |
| structured error | 42 | 38 |
| tabular task list | 32 | 21 |
| Aggregate | 181 | 160 (88%) |

Observations, for these samples only:

- Object-dominated views save little because JSON's structural overhead is
  small relative to the repeated keys.
- Tabular arrays carry the real savings (here about 34% fewer tokens) because
  keys are declared once and rows omit them.
- TOON never costs more than compact JSON for these samples; a regression
  guard in the test fails if it would.
