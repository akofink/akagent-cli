# Optional `akagent` integration instruction

The following snippet is intended for an operator-approved addition to global or `~/dev` agent guidance.

> Agents must not invoke `akagent` as an automated integration unless `AKAGENT_ENABLED=1` is present in the invoking environment.
> Treat a missing signal or any other value as disabled, and continue without the integration.
> The read-only `akagent integration inspect` command reports the current gate state.
> Direct human `akagent` commands remain available regardless of the gate.

Enable the integration for the current shell with:

```bash
export AKAGENT_ENABLED=1
```

Disable it again with:

```bash
unset AKAGENT_ENABLED
```
