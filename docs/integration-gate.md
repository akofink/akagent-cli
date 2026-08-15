# `akagent` integration gate

Automated integrations must remain disabled unless the invoking environment contains exactly `AKAGENT_ENABLED=1`.
A missing signal, an empty value, or any other value is disabled.

Inspect the current state without changing it:

```bash
akagent integration inspect
```

The command reports the signal name, whether the gate is enabled, and a non-secret reason.
Direct human `akagent` commands are available regardless of the gate.

Enable the gate for the current shell only when an approved integration needs it:

```bash
export AKAGENT_ENABLED=1
```

Disable it immediately when the integration is no longer needed:

```bash
unset AKAGENT_ENABLED
```

Integrations must check the gate before invoking automated `akagent` behavior.
They must continue without the integration when the gate is disabled.
The gate does not grant credentials, launch a managed agent, or change direct CLI behavior.
