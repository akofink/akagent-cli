# Roadmap

This roadmap describes the durable registry after issue #140.
The core prioritizes work that survives an agent, terminal, provider, or network failure.

## Shipped

- Durable task, repository, resource, execution, event, archive, observation, checkpoint, disposition, recovery, and delivery records.
- Deterministic in-flight, attention, maintenance, deferred, and history views.
- Store-only reconciliation and explicit legacy migration.
- Provider-neutral session references and metadata-only evidence.
- Worker protocol version `2` with declarative capabilities.
- Structured pre-store refusal for removed orchestration and credential command families.
- Offline subprocess canaries for retained command paths.

## Current boundary

`akagent` records intent and caller-submitted facts.
It does not launch or stop processes, inspect tmux or PIDs, run Git, create or remove worktrees, resolve credentials, parse provider sessions, capture terminal output, or call a forge.
External tools and skills own those side effects when they remain needed.
Deployment and credential capabilities may be intentionally retired.

The default inventory is the in-flight view.
Attention and maintenance views expose recovery and cleanup debt without hiding unfinished work.
The core preserves missing, stale, unavailable, and contradictory facts.
It never infers completion or creates duplicate work during recovery.

## Follow-ups

1. Verify integrated CLI, skills, and active-agent compatibility on `main` before changing the installed binary.
2. Add external-tool adapters only where a real workflow requires them.
3. Keep provider-native session discovery and transcript interpretation outside the core.
4. Improve archive and checkpoint backup procedures without expanding the local store into a daemon or central service.
5. Add protocol-version migration guidance when future breaking lifecycle changes are proposed.

## Non-goals

- A daemon or central scheduler.
- Remote execution or cross-machine synchronization.
- Provider transcript indexing or private context persistence.
- Feature-parity replacements for intentionally retired deployment or credential capabilities.
