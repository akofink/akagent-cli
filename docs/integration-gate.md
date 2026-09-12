# Optional integration compatibility signal

Direct agent and human use of the durable `akagent` CLI is the normal workflow.
The signal applies only to optional automation that needs an environment-level compatibility switch.
It is not a prerequisite for direct record commands.

`AKAGENT_ENABLED` is the immediate per-environment disable signal.
Automation is considered enabled unless the variable is set to the exact value `0`.
The core does not launch automation or providers based on this signal.

Inspect the signal without opening the state store or changing records:

```bash
akagent integration inspect
```

Disable optional automation for the current shell:

```bash
export AKAGENT_ENABLED=0
```

Re-enable it:

```bash
unset AKAGENT_ENABLED
```

The signal does not grant credentials, launch tasks, or authorize external side effects.
The removed `integration launch` command returns a structured usage error with exit code `2` before store access.
Direct task, resource, execution, checkpoint, publication, archive, and reconciliation commands remain available regardless of the signal.
