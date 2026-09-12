# Credential metadata boundary

## Core behavior

The `akagent` core does not provide a credential command family or a credential value-resolution subsystem.
It never reads, validates, prints, injects, rotates, or deletes credential values.
External tools and skills own credential readiness, injection, rotation, and cleanup.

Task, resource, execution, and archive records may preserve historical credential IDs, source references, requirements, warnings, and cleanup debt.
These fields are opaque historical metadata and are never resolved or erased by record inspection or migration.

## Legacy records

Storage schema version `1` remains readable after orchestration removal.
Legacy credential-reference and debt fields remain available to recovery and maintenance views.
Legacy unfinished or stopped work must use an explicit store-only migration or adoption path.
Missing credentials and missing processes never prove completion or trigger implicit reactivation.

## Removed commands

The former credential command family, including list, inspect, doctor, and clean, is removed.
Each recognized command returns the structured usage error contract with exit code `2` before opening or mutating the state store.
The error names only the `credential` command family and provides safe migration guidance.

External tools may inspect credential readiness and perform cleanup separately.
They must submit only redaction-safe observations or recovery debt through the durable record boundary.

## Security contract

Credential values must not appear in TOON output, errors, logs, events, prompts, process arguments, session references, or diagnostics.
Provider files and terminal content remain outside the core and are never opened by record inspection, archive, reconciliation, or migration.
