# Agent Guidance

- Read the assigned GitHub issue and `docs/handoff.md` before implementation. Load the repository skill `developing-akagent` for issue delivery and protocol checks.
- Use one dedicated issue branch and worktree based on current `origin/main`; never implement directly in the primary `main` checkout.
- Keep changes inside the issue's stated scope and file ownership. Do not refactor another parallel issue's integration surface opportunistically.
- Keep the installed binary name `akagent`; `aka` is an optional shell alias, not a second binary.
- Treat the CLI protocol as the stable product boundary.
- Emit concise TOON on stdout for data and structured errors.
- Send opt-in diagnostics to stderr and never mix progress with protocol output.
- Never emit credential values in output, errors, logs, fixtures, process arguments, or committed files.
- Preserve local tmux and Git worktree recovery paths.
- Do not add remote execution, containers, a daemon, or a central store without a demonstrated requirement.
- Run `go test ./...`, `go test -race ./...`, and `go vet ./...` before committing.
- Use one signed Conventional Commit and include `Fixes #N` for the assigned implementation issue. Push and open a pull request when requested, but never merge without explicit operator instruction.
