# Roadmap

This document is the single source of truth for shipped capabilities and prioritized follow-up work.
The core prioritizes records that survive an agent, terminal, provider, or network failure.

The project follows a source-tracking release policy: users build from a checkout, and `akagent update` fast-forwards that checkout to `origin/main`.
There is no separately versioned binary release channel.

## Shipped

- Durable task, repository, resource, execution, event, archive, observation, checkpoint, disposition, recovery, and delivery records.
- Explicit caller-owned `task external` create, finish, and archive commands alongside managed record commands.
- Deterministic in-flight, attention, maintenance, deferred, and history views.
- Store-only reconciliation and explicit legacy migration.
- Provider-neutral session references and metadata-only Phase 0 evidence views.
- Human terminal presentation for `task list` and `task inspect`, plus JSON list and inspect output.
- Worker protocol version `2` with declarative `registry`, `checkpoint`, `observation`, and `handoff` capabilities.
- Structured pre-store refusal for removed orchestration and credential command families.
- Offline subprocess canaries for retained command paths.
- Check CI runs race tests, vet, `staticcheck`, `errcheck`, and `govulncheck`, with Go 1.25 and commit-pinned Actions; branch protection requires the Check workflow.
- A merged-coverage workflow reports package and total coverage and enforces 73.8% overall and 72.2% for `internal/store`, backed by additional archive, recovery, and external-record durability tests.
- Fuzz targets exercise TOON encoding and task, execution, and resource ID validation.
- Focused files separate task command handlers and external-record store operations along existing boundaries.
- External-record rollback failures are reported as partial outcomes, and lifecycle errors use typed categories instead of message matching.
- `akagent update` subprocesses have a five-minute timeout, and `worker inspect` reports the build revision.

## Current boundary

`akagent` records intent and caller-submitted facts.
It does not launch or stop processes, inspect tmux or PIDs, run Git, create or remove worktrees, resolve credentials, parse provider sessions, capture terminal output, or call a forge.
External tools and skills own those side effects when they remain needed.
Deployment and credential command families stay retired.

The default inventory is the in-flight view.
Attention and maintenance views expose recovery and cleanup debt without hiding unfinished work.
The core preserves missing, stale, unavailable, and contradictory facts.
It never infers completion or creates duplicate work during recovery.
TOON remains the machine-readable protocol on stdout.
Human output is an explicit terminal presentation, not a parsing interface.

## Follow-ups

Prioritized from the current direct coding-agent workflow:

1. Roll out derived resource state in the phases defined by [Derived state](derived-state.md), so agents stop hand-recording Git, forge, terminal, and provider facts.
2. Verify integrated CLI, skills, and active-agent compatibility on `main` before changing the installed binary.
3. Keep managed versus caller-owned external provenance distinct in agent workflows so `--target external` on a managed execution is not treated as `task external`.
4. Add later session-evidence adapter phases only where a real workflow needs native discovery beyond metadata-only references.
5. Improve archive and checkpoint backup procedures without expanding the local store into a daemon or central service.
6. Treat additional machine-readable presentation formats as optional follow-ups.
   Do not replace TOON as the protocol boundary or human output as the terminal view.
7. Add protocol-version migration guidance when a future breaking lifecycle change is proposed.

## Non-goals

- A daemon or central scheduler.
- Remote execution or cross-machine synchronization.
- Provider transcript indexing or private context persistence.
- Feature-parity replacements for intentionally retired deployment or credential capabilities.
- Recreating launch, attach, stop, Git, tmux, or cleanup orchestration in the core.
