# Agent Guidance

- Read the assigned GitHub issue and `docs/handoff.md` before implementation. Load the repository skill `developing-akagent` for issue delivery and protocol checks.
- Use one dedicated issue branch and worktree based on current `origin/main`; never implement directly in the primary `main` checkout.
- Keep changes inside the issue's stated scope and file ownership. Do not refactor another parallel issue's integration surface opportunistically.
- Keep the installed binary name `akagent`; `aka` is an optional shell alias, not a second binary.
- Treat the CLI protocol as the stable product boundary.
- Emit concise TOON on stdout for data and structured errors.
- Send opt-in diagnostics to stderr and never mix progress with protocol output.
- Never emit credential values in output, errors, logs, fixtures, process arguments, or committed files.
- Treat tmux, Git, worktrees, providers, credentials, and forge clients as external tools; record their redaction-safe observations without requiring them for durable inspection or recovery.
- Do not add remote execution, containers, a daemon, or a central store without a demonstrated requirement.
- Run `go test ./...`, `go test -race ./...`, and `go vet ./...` before committing.
- Use the record-only `akagent` lifecycle directly for durable task, resource, execution, checkpoint, disposition, publication, completion, archive, and recovery facts.
- If a command may have mutated durable state and fails, inspect the affected task and reconcile before retrying; never launch a replacement or infer completion from a missing process.
- For explicitly requested issue delivery, this repository authorizes issue creation, branch pushes, and merging the issue branch to `main` without a pull request, after `go test ./...`, `go test -race ./...`, and `go vet ./...` pass locally. Fast-forward when possible; stop on divergence or conflict. Do not open a pull request unless asked. Higher-level safety rules still apply.
- Use one signed Conventional Commit and include `Fixes #<issue>` when delivering a GitHub issue.
- Do not install the binary until integrated CLI, lifecycle skill, and active-agent compatibility are verified on `main`.

## Cloud session start

In a cloud session (`CLAUDE_CODE_REMOTE=true` or `CODEX_ENVIRONMENT_ID` set), before other work bring the attached checkouts up to date and run the cloud setup, then read `agents/global-personal.md` in the notes checkout. This block is self-contained on purpose so a stale checkout can bootstrap itself; keep it stable.

```sh
for r in /workspace/*/ /home/user/*/; do [ -e "$r/.git" ] && [ -z "$(git -C "$r" status --porcelain)" ] && git -C "$r" fetch -q origin main && git -C "$r" merge -q --ff-only origin/main; done
bash "$(ls -d /workspace/dots /home/user/dots 2>/dev/null | head -1)/bin/cloud-session-setup.sh"
```
