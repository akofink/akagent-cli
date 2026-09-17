# Quick start

This guide uses only public commands and generic local paths.
`akagent` is a local-first durable registry CLI for task, resource, execution, observation, checkpoint, and recovery records.
The core does not launch processes, inspect tmux, run Git, mutate worktrees, resolve credentials, parse provider sessions, or capture terminal output.
External tools and skills own those side effects when they remain needed.

## Install or update

Build the binary from a public source checkout:

```bash
git clone https://github.com/akofink/akagent-cli.git /path/to/akagent-cli
cd /path/to/akagent-cli
go build -o "$HOME/.local/bin/akagent" ./cmd/akagent
```

Update from a clean checkout on `main` with an explicit source path:

```bash
akagent update --source /path/to/akagent-cli
```

The update command is an explicit installation operation outside task lifecycle.
It may use Git and replace the installed binary atomically.
Do not use it as a task or recovery command.

## Create durable records

Repository registration records identity without checking or changing the checkout:

```bash
akagent repository register demo /path/to/checkout --policy direct
akagent repository inspect demo
```

Create a task and record a resource without Git or filesystem access:

```bash
akagent task create --title "Review the build" --task-id review-build
akagent task resource create review-build \
  --resource-id app-resource --repository demo \
  --branch akofink/review-build --base base-revision --head head-revision \
  --worktree /path/to/worktree
```

The repository, branch, revision, and worktree values are durable declarations.
External tools validate and mutate actual checkouts.

Create a tool-neutral execution without starting it:

```bash
akagent task execution create review-build \
  --execution-id review-attempt --target external --command /path/to/tool \
  --resource app-resource
```

The command and target are metadata only.
No process, provider, credential, Git, or tmux operation is implied.
`--target external` on this managed command does not create a caller-owned external execution.
Use the `task external` family when the caller needs owned provenance, operation IDs, and revision-checked completion.
See [record-only-lifecycle.md](record-only-lifecycle.md).

## Inspect and publish

```bash
akagent task list
akagent task inspect review-build
akagent task publish review-build --condition active --activity "running tests"
akagent task execution publish review-build review-attempt \
  --condition waiting --reason "external review" --activity "awaiting result"
akagent task reconcile review-build
```

Inspection and reconciliation remain usable offline.
They preserve unavailable, stale, missing, and contradictory observations rather than inferring completion.

The default `task list` view is `in-flight`.
Use `--view attention`, `--view maintenance`, `--view deferred`, `--view history`, or `--all` for other deterministic views.
TOON is the machine-readable protocol on stdout.
Use `--format json` for compact JSON interchange of the same typed views.
Use `--format human` only for direct terminal presentation.

## Record external observations and sessions

External tools submit observations and session references without provider content:

```bash
akagent task execution session add review-build review-attempt \
  --tool example-tool --session-id session-123 \
  --reference-path /path/to/session-record
akagent task resource update review-build app-resource \
  --metadata delivery=published \
  --external-url https://forge.example/pull/78
```

The core validates non-secret reference shape and local path metadata only.
It never opens or parses provider session files.

## Checkpoints and completion

```bash
akagent task checkpoint write review-build \
  --idempotency-key checkpoint-1 --expected-revision 1 \
  --task-kind implementation --completion-contract "verified delivery" \
  --next-action "inspect the pull request and required checks"
akagent task checkpoint inspect review-build
akagent task finish review-build succeeded "verified delivery"
akagent task archive review-build
```

Completion is an explicit declaration against the caller's contract.
A missing process, checkout, provider session, or credential never proves success.
Archives contain durable records, event history, checkpoints, and caller-submitted facts without terminal capture or host inspection.

Legacy unfinished and stopped tasks require explicit store-only migration or adoption.
The migration preserves task and resource IDs and historical Git and session facts.
It does not create duplicate tasks or worktrees and does not implicitly reactivate work.
Storage schema version `1`, legacy manifests, archives, events, credential metadata, and recovery debt remain readable.

## Removed command families

Launch, attach, stop, deployment, cleanup, credential, provider orchestration, and the transitional `task record` family are removed.
Recognized public and hidden forms return structured usage errors with exit code `2` before opening or mutating the state store.
The error names only the command family and gives safe migration guidance.

`akagent integration inspect` remains a read-only `AKAGENT_ENABLED` compatibility signal for optional automation.
`akagent worker inspect` reports protocol version `2` and declarative `registry`, `checkpoint`, and `observation` capabilities.
