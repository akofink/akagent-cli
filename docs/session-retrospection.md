# Durable agent-session retrospection

## Status and decision

This proposal addresses issue [#121](https://github.com/akofink/akagent-cli/issues/121).

The recommended design is local-first, opt-in capture built around provider-native session references and small, redacted derived records.

`akagent` owns the relationship between a task, resource, execution, and evidence record, but it does not own or parse provider transcript formats.

Raw session content remains in the provider's local storage unless an operator explicitly chooses a separate local export.

A retrospective must distinguish available evidence from inference and unavailable history rather than presenting a complete-looking timeline.

## Problem and boundaries

A task event proves that an `akagent` operation happened.

It does not prove what an agent did between two CLI calls, what happened in a provider UI, or what a shell process printed.

Tmux scrollback and process state can help while a window exists, but they are not durable history.

The design covers Claude Code, Codex CLI, Pi, and OpenCode, plus optional shell and terminal sources.

It does not add a daemon, remote execution, a central store, mandatory provider dependencies, credential collection, or silent upload.

It does not promise reconstruction of history that a provider did not save or that an operator did not enable capturing.

## Evidence model

Every fact shown by retrospection carries a provenance and confidence class.

| Class | Meaning | Examples |
| --- | --- | --- |
| `recorded` | Durable fact written by `akagent` or a capture adapter. | Task event, execution status, provider session ID, checkpoint cursor. |
| `observed` | Fact read from a local provider, process, tmux, Git, or terminal source. | A native transcript exists, a process exited, `HEAD` changed. |
| `inferred` | A bounded conclusion derived from recorded or observed facts. | "The session likely stopped after the last checkpoint." |
| `unavailable` | History cannot be established from retained sources. | A provider file was deleted, capture was disabled, or a disconnect occurred before the first checkpoint. |

The CLI must show the class next to summaries and must not turn an inference into an event.

The minimum retrospective timeline combines these independent streams:

1. `akagent` task, resource, and execution events.
2. Process, tmux, and Git observations already used by reconciliation.
3. Provider-native session metadata and transcript indexes when an adapter is enabled.
4. Optional shell or terminal observations when the operator explicitly enabled them.

The streams are correlated by task ID, execution ID, absolute worktree identity, provider name, provider session ID, and timestamps.

Timestamp ordering is advisory because clocks, buffering, and provider write delays can differ.

An archive stores the correlation metadata and evidence manifest, not an implicit claim that every source was captured.

## Native provider evidence

Provider adapters should use documented CLI resume or export behavior and treat on-disk layouts as versioned implementation details.

The provider session ID is the stable join key; a filename or database row is only a local reference.

| Provider | Native identity and local artifact | Native recovery surface | Adapter boundary |
| --- | --- | --- | --- |
| Claude Code | A UUID session ID is used by the resume flow and is commonly represented by a JSONL transcript below `~/.claude/projects/` for the encoded workspace. | `claude --resume <session-id>` or `--continue`. | Record the session ID, workspace mapping, artifact reference, provider version, and last readable cursor. Do not assume every JSONL line is public or stable. |
| Codex CLI | A UUID session ID is used by `codex resume`; local rollout JSONL artifacts are commonly stored below `$CODEX_HOME/sessions/` in date-partitioned paths. | `codex resume <session-id>` or `--last`. | Prefer the CLI's session identity and export or inspection behavior over SQLite or rollout filename parsing. Record whether the session was interactive or non-interactive when exposed. |
| Pi | A project-scoped session ID is present in the session filename and can be selected with `--session`, `--resume`, or `--continue`; default sessions are JSONL below the Pi session directory. | `pi --session <path-or-id>` or `pi --resume`. | Record the exact session ID and project/workspace mapping, while treating JSONL event shapes as Pi-owned. A fork creates a new session that links to its parent when that relationship is available. |
| OpenCode | A `ses_...` session ID is used by the session commands and `-s/--session`; local session data is stored under the platform data directory and may be split across provider-owned records. | `opencode --session <session-id>` and `opencode export <session-id>`. | Use the OpenCode CLI for export or inspection where possible. Do not couple the core store to the current storage directory layout or its internal message and part records. |

These artifact locations and formats can change with provider versions, custom home directories, platform conventions, and cleanup settings.

An adapter records the provider version and discovery method so later inspection can explain which interpretation was used.

A missing or unreadable artifact leaves the reference historically valid and changes its evidence state to `unavailable`.

The adapter must never place transcript text, tool arguments, model output, or environment values in an `akagent` event merely to make inspection convenient.

## Provider-neutral capture contract

The existing execution session reference is the identity layer and remains provider-neutral.

A future capture record associated with an execution should contain only non-secret metadata:

```text
capture_id
execution_id
source_kind              native_reference | adapter_summary | shell_audit | terminal_capture
provider                 claude | codex | pi | opencode | zsh | terminal | other
provider_session_id
parent_session_id        optional fork or resume relationship
workspace_identity       repository and worktree identity, not prompt text
started_at
last_seen_at
ended_at                 optional
state                    pending | active | disconnected | complete | expired | unavailable
coverage                 named capabilities, not a completeness claim
artifact_reference       local provider-owned path or opaque local URI
artifact_version         provider and adapter versions
cursor                   provider-local checkpoint or offset, if safe
redaction_policy
retention_class
integrity                 optional size, digest, or last-observed marker
error_category            optional redaction-safe discovery error
```

`provider_session_id` is required once the provider exposes one.

`artifact_reference` is a pointer and never a copy of the artifact.

An adapter may add a short redacted checkpoint summary or searchable index entry, but each derived value must retain its source cursor and evidence class.

The contract must support multiple sessions per execution because a provider resume, fork, or replacement session is not the same session.

The idempotency key is `(execution_id, provider, provider_session_id, cursor)`.

Recommended append-only events are `session_discovered`, `session_checkpointed`, `session_disconnected`, `session_reconciled`, and `session_expired`.

Events contain IDs, timestamps, state, coverage, and redaction outcomes, never raw session content.

The task and execution archives include capture manifests and event history, while raw provider artifacts follow their own lifecycle.

## What to capture

The default is metadata-only discovery.

A provider adapter may opt in to local summaries or indexes after the operator enables that source for the task or repository.

The preferred progression is:

1. Discover and record the native session ID and local reference.
2. Record lifecycle checkpoints, provider version, workspace mapping, and safe counts such as message or tool-event counts when the provider exposes them.
3. Generate a redacted summary or local index for inspection without copying the raw transcript into the task store.
4. Offer an explicit local export for an operator who needs raw content.

Capture is best effort and must not make an otherwise successful agent execution fail.

A checkpoint failure records retryable discovery debt and the last known cursor.

The adapter may retry from that cursor, but it must not claim coverage for a gap it could not read.

## Shell logging evaluation

Zsh command history is useful corroborating evidence, but it is not the primary source of agent activity.

It generally records accepted interactive commands, not command output, tool calls made inside an agent, provider UI actions, or every subprocess.

History can be delayed, merged, disabled, rewritten by shell options, affected by multiline editing, or absent for non-interactive shells.

`EXTENDED_HISTORY` timestamps improve correlation but do not establish start and completion boundaries.

Commands can contain API keys, tokens, passwords, personal data, or sensitive arguments, and history files are often shared across unrelated work.

A zsh hook also has portability and interaction risks because it changes startup files, command latency, shell widgets, and behavior across nested or non-default shells.

Therefore zsh history is an explicitly opt-in, per-workspace corroborating source with redaction before persistence.

The recommended default records command metadata such as a timestamp, working-directory identity, exit status when available, and a keyed digest rather than the command text.

Raw command text requires a separate explicit policy and must never be copied into task events by default.

The source must declare its blind spots and must not label history absence as proof that no work occurred.

## Alternative comparison

| Alternative | Strength | Failure or privacy boundary | Decision |
| --- | --- | --- | --- |
| Native session references | Stable resume identity, low storage cost, no transcript copying. | Requires the provider to save a session and an adapter to discover it. | Foundation and default. |
| Provider adapters that summarize or index logs | Answers more historical questions and can remain local. | Format churn, parsing complexity, prompt and tool secrecy, incomplete checkpoints. | Add incrementally behind explicit local opt-in. |
| Zsh audit logging | Captures some activity outside provider APIs. | Incomplete interactive history, high secret leakage risk, shell customization and portability concerns. | Optional corroboration only. |
| Terminal capture | Preserves visible input and output across arbitrary tools. | Captures secrets and unrelated work, high volume, terminal escape and redaction problems. | Optional last-resort source with narrow scope and aggressive retention. |
| Copy all provider logs into `akagent` | Convenient single archive. | Duplicates sensitive data, couples the core to providers, and expands access and deletion obligations. | Reject. |

## Security, retention, and access

Capture is disabled unless enabled for a source, and an enablement decision is recorded without recording its secret-bearing configuration.

All local metadata uses the existing restrictive worker-local store permissions and descriptor-safe reads.

Provider artifacts remain under provider ownership and inherit the provider's local permissions.

No provider session content, credentials, environment dumps, authorization headers, or secret command arguments may enter task events, manifests, archives, protocol output, process arguments, or diagnostics.

Redaction runs before a summary, index, digest input, or export is persisted.

It should cover known credential formats, configured secret values held in memory, authorization headers, common private-key blocks, and provider-specific sensitive fields.

Redaction is conservative and visible: inspection reports the policy and whether content was removed, but never reports the removed value.

Redaction is not a proof of safety, so raw capture remains opt-in and local.

The default retention class is `metadata`, retained with the task archive while the task is useful.

`summary` and `index` retention must have explicit durations and an operator deletion command.

Raw exports are never retained by `akagent` by default and are deleted only through the provider or the operator's explicit local policy.

An archive preserves references to missing or expired artifacts as `unavailable` rather than failing or silently removing history.

There is no network sink in this design.

Any future synchronization must be a separate, visible, authenticated feature with an explicit destination and consent.

## Disconnect and recovery semantics

The adapter records `pending` before launching a provider when possible, then changes it to `active` after discovering the provider session ID.

If the provider exposes the ID only after startup, the initial execution remains useful without a session reference and the adapter records the reference at the first safe opportunity.

A tmux or provider disconnect triggers normal `akagent` process and window reconciliation first.

If the process is gone but the native artifact has a final readable checkpoint, the capture becomes `complete` or `disconnected` according to the provider's evidence.

If the process is gone and the final boundary is unknown, the capture remains `disconnected` and the retrospective says that the end is inferred.

A resumed or forked provider session creates another capture record linked to the prior session and does not overwrite prior evidence.

A missing file, provider cleanup, parse failure, or unsupported provider version produces `unavailable` evidence with redaction-safe recovery guidance.

Reconciliation never deletes provider artifacts, guesses missing transcript events, or marks an execution successful from session output alone.

A capture adapter can be restarted without a daemon because checkpoints, cursors, and idempotency keys are durable.

## Inspection UX

Dedicated evidence views show metadata-only evidence state without exposing content.

The Phase 0 focused surface is read-only:

```text
akagent task execution evidence list <task-id> <execution-id>
akagent task execution evidence inspect <task-id> <execution-id> <capture-id>
```

The list view derives one metadata-only capture from each existing provider-neutral session reference.
It shows the capture ID, source kind, provider, provider session ID, state, coverage, retention class, and evidence class.
It reports an execution with no session references as `unavailable` with reason `no_session_references`, which is distinct from a session reference whose local artifact path later becomes unavailable.

The inspect view shows the artifact reference, artifact state, redaction policy, retention class, redaction-safe error category, and recovery guidance.
Phase 0 does not read provider-owned files beyond safe local path metadata and does not parse provider transcript formats.
A reference with an existing regular local artifact is `available` with evidence class `observed`.
A reference with no local path is `unknown` with evidence class `recorded`.
A reference with a missing, unreadable, symlink, directory, or non-regular path is `unavailable`.

Future phases may add adapter-owned reconciliation, provider versions, cursors, safe counts, provider-specific resume suggestions, and derived summaries behind explicit opt-in.
The default evidence views never print raw content and have no implicit `upload`, `sync`, or export action.

A future explicit `--export <local-path>` operation must confirm scope, apply redaction, refuse overwrites by default, and report exactly which evidence was unavailable.

All output remains deterministic TOON on stdout, with optional human-readable diagnostics on stderr.

## Phased implementation plan

### Phase 0: contract and evidence views

Document the capture schema, evidence classes, redaction policy, retention classes, and migration rules.

Add synthetic fixtures and conformance tests for multiple sessions, gaps, stale references, forks, and missing artifacts without committing real transcripts.

Add read-only inspection of existing session references and explicit unavailable or unknown states.

### Phase 1: native reference adapters

Implement opt-in local discovery adapters for Claude Code, Codex CLI, Pi, and OpenCode.

Record provider versions, session IDs, workspace identity, artifact references, cursors, and lifecycle checkpoints.

Keep adapters outside the core lifecycle and preserve direct human and shell execution when a provider is absent.

### Phase 2: local summaries and indexing

Add provider adapters that produce redacted, bounded summaries and indexes from local artifacts.

Make parsing version-aware, resumable, idempotent, and best effort.

Expose retention and deletion controls and test that raw content never reaches core events or default protocol output.

### Phase 3: optional shell and terminal sources

Add narrowly scoped zsh history correlation first, with explicit configuration and command metadata redaction.

Evaluate terminal capture only after measuring redaction quality, storage volume, performance, and cross-terminal behavior.

Keep both sources disabled by default and clearly mark their incomplete coverage.

### Phase 4: recovery and operational hardening

Test disconnects, provider upgrades, provider cleanup, interrupted indexing, clock skew, permission failures, archive and cleanup, and concurrent inspection.

Document operator consent, retention review, artifact deletion, and recovery procedures for each provider.

## Acceptance criteria

1. The public contract represents native references, optional derived evidence, provenance, coverage, redaction, retention, cursors, and unavailable history without provider-specific fields in the core execution record.
2. Claude Code, Codex CLI, Pi, and OpenCode each have a documented stable session-ID discovery and resume or export path with versioned adapter behavior.
3. A provider or tmux disconnect leaves task, execution, capture metadata, and last checkpoint inspectable without requiring the provider or a live terminal.
4. A fork, resume, missing artifact, parse gap, and expired artifact are represented without overwriting earlier evidence or inventing events.
5. Raw transcripts, terminal output, credentials, environment values, and secret command arguments are absent from default `akagent` records, archives, TOON output, diagnostics, and process arguments.
6. Capture is opt-in per source, local by default, best effort, retryable, and never silently uploads content or fails unrelated task lifecycle operations.
7. Inspection distinguishes recorded, observed, inferred, and unavailable facts and provides safe provider-native recovery suggestions.
8. Retention, deletion, permissions, redaction outcomes, and access boundaries are covered by tests and operator documentation.
9. Synthetic conformance tests demonstrate idempotent checkpoint replay, concurrent updates, archive preservation, and recovery after partial writes or adapter interruption.

## References

- [Claude Code documentation](https://docs.anthropic.com/en/docs/claude-code)
- [Codex repository and CLI](https://github.com/openai/codex)
- [Pi repository and documentation](https://github.com/badlogic/pi-mono)
- [OpenCode CLI documentation](https://opencode.ai/docs/cli/)
- [`akagent` protocol](protocol.md)
- [Worker-local state store](storage.md)
- [Task CLI contract](task-cli.md)
