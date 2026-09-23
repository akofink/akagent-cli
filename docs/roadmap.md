# Roadmap

This roadmap describes the shipped protocol-v2 record-only registry and the next credible work.
The core prioritizes records that survive an agent, terminal, provider, or network failure.

## Shipped

- Durable task, repository, resource, execution, event, archive, observation, checkpoint, disposition, recovery, and delivery records.
- Explicit caller-owned `task external` create, finish, and archive commands alongside managed record commands.
- Deterministic in-flight, attention, maintenance, deferred, and history views.
- Store-only reconciliation and explicit legacy migration.
- Provider-neutral session references and metadata-only Phase 0 evidence views.
- Human terminal presentation for `task list` and `task inspect`.
- Worker protocol version `2` with declarative `registry`, `checkpoint`, `observation`, and `handoff` capabilities.
- Structured pre-store refusal for removed orchestration and credential command families.
- Offline subprocess canaries for retained command paths.

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

1. Verify integrated CLI, skills, and active-agent compatibility on `main` before changing the installed binary.
2. Keep managed versus caller-owned external provenance distinct in agent workflows so `--target external` on a managed execution is not treated as `task external`.
3. Add later session-evidence adapter phases only where a real workflow needs native discovery beyond metadata-only references.
4. Improve archive and checkpoint backup procedures without expanding the local store into a daemon or central service.
5. Treat additional machine-readable presentation formats as optional follow-ups.
   Do not replace TOON as the protocol boundary or human output as the terminal view.
6. Add protocol-version migration guidance when a future breaking lifecycle change is proposed.

## Non-goals

- A daemon or central scheduler.
- Remote execution or cross-machine synchronization.
- Provider transcript indexing or private context persistence.
- Feature-parity replacements for intentionally retired deployment or credential capabilities.
- Recreating launch, attach, stop, Git, tmux, or cleanup orchestration in the core.
