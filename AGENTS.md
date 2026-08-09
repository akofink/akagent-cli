# Agent Guidance

- Use dedicated worktrees for changes after the initial repository bootstrap.
- Keep the installed binary name `akagent`; `aka` is an optional shell alias, not a second binary.
- Treat the CLI protocol as the stable product boundary.
- Emit concise TOON on stdout for data and structured errors.
- Send opt-in diagnostics to stderr and never mix progress with protocol output.
- Preserve local tmux and Git worktree recovery paths.
- Do not add remote execution, containers, a daemon, or a central store without a demonstrated requirement.
- Run `go test ./...` and `go vet ./...` before committing.
