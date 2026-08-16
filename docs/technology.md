# Technology evaluation

## Criteria

The implementation needs:

- A fast single executable with straightforward installation.
- Linux and macOS development, with Linux workers first.
- Reliable subprocess, signal, terminal, filesystem, and permission control.
- Concurrent status queries and future network transports.
- Typed protocol records and versioned schemas.
- Conforming TOON output.
- Deterministic tests for failure and idempotency.
- Low runtime and token overhead.

Correctness, recovery, and maintainability matter more than initial implementation speed.

## Go

Go offers simple cross-compilation, a strong standard library for systems work and networking, goroutines and contexts for bounded concurrency, fast startup, typed protocol records, and straightforward black-box testing.

Risks include careful pseudo-terminal work, potentially repetitive typed errors, and an immature TOON ecosystem that must be checked against the current specification.

Go is the selected initial language.

## Rust

Rust offers strong ownership and concurrency guarantees, excellent performance, and precise error modeling.
Its implementation and review complexity would slow experimentation while command semantics are still changing.
It becomes more attractive if a large service, high concurrency, or stronger in-process safety becomes central.

## TypeScript and Node.js

TypeScript provides rapid CLI development, strong schema tooling, and direct access to the reference TOON ecosystem.
It also fits future JavaScript-based agent plugins.

The durable worker executable would inherit Node runtime, packaging, startup, dependency, signal, and pseudo-terminal complexity.
TypeScript remains suitable for thin integrations rather than the core.

## Python

Python is effective for disposable prototypes but makes interpreter, dependency, packaging, startup, and environment drift part of worker bootstrap.
It is not the preferred durable implementation.

## Shell

Shell directly reaches tmux and Git and remains useful for setup and focused tests.
Weak structured-data handling, locking, signal control, quoting, and partial-failure behavior make it unsuitable as the protocol implementation.

## Implementation boundaries

- Use typed internal values rather than TOON-shaped strings.
- Treat TOON as output and interchange; decide persistence separately.
- Begin with worker-local files, locks, and atomic replacement.
- Use local function calls and command execution for the current worker.
- Keep the implementation centered on the local CLI, worker-local files, and direct command execution.

## Validation status and remaining work

- TOON 4.1 output is validated against official fixtures and representative token measurements.
- Tmux launch, task identity, verified attachment, stop, and process observation have focused tests.
- Concurrent state publication and inspection use per-task locks and race-enabled tests.
- Atomic manifest replacement and recoverable event recording have focused tests.
- Credential readiness checks avoid reading or printing source values.
- The optional managed local Pi execution integration validates prompt-file references, safe environment construction, process replacement, retry, and process identity on top of generic executions.
- Terminal resize and cross-compilation remain outside the current CLI contract.
