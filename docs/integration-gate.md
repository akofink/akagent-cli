# Optional integration compatibility signal

Direct agent and human use of the stable `akagent` CLI is the normal local workflow.
The integration signal applies only to optional automated integrations that need an environment-level compatibility switch.
It is not a prerequisite for direct task, resource, or execution commands.

`AKAGENT_ENABLED` is the immediate per-environment disable signal for optional automated integrations.
At the CLI boundary, optional automation is enabled unless `AKAGENT_ENABLED` is set to the exact value `0`.

Inspect the current state without changing it:

```bash
akagent integration inspect
```

Disable automation immediately for the current shell with:

```bash
export AKAGENT_ENABLED=0
```

Re-enable automation for the current shell with:

```bash
unset AKAGENT_ENABLED
```

After a command that may have mutated task state fails, inspect the task and run reconciliation before attempting a manual fallback.
The signal does not grant credentials or launch tasks by itself.
The optional provider-neutral `akagent integration launch` command is an automated workflow entry point and checks the signal before opening the state store or creating an execution.
When disabled, it returns a skipped success without lifecycle side effects.
When enabled, it records and launches a generic `workflow` execution through the normal task lifecycle.
The signal also does not control direct CLI commands or optional execution providers.
Direct agent and human commands remain available regardless of the signal.
Core task, resource, and generic execution commands remain available when Pi is not installed.
An explicit direct shell launch remains available regardless of the signal.
Pi is an optional execution integration selected explicitly with `akagent task launch --target pi`.
