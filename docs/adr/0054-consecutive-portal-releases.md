# Consecutive Portal releases over one Practice cohort

## Context

ADR-0053's first Portal-only successor is active. A later frontend correction
must retain that Portal as its immediate rollback point, while leaving the
Practice cohort, current mail configuration and deferred Career/getWork intact.
The original entry deliberately rejects this state; its receipt must not be
rewritten to pass the old gate.

## Decision

Extend the same fixed Portal-only entry with `henukit-portal-successor-v2`
receipts. Accept the original Practice predecessor or a linear chain of active
Portal v1/v2 receipts ending at that same exact five-service Practice format.
Validate every retained receipt's canonical path, SHA, private ownership,
runtime/helper/engine/Compose identities, baseline and predecessor digest/binding.
Reject repeats, cycles, unexpected formats/scopes and chains longer than 64.

The current Portal must match the immediate predecessor image. The four other
Practice images must still match the root Practice receipt, and all non-Portal
containers must match each retained Portal receipt. Historical Practice global
environment snapshots remain historical: each new release captures and backs
up the current environment without restoring an old mail configuration.

Use the existing exact-main Actions provenance, archive/platform verification,
disk floor, watcher handshake and locks. Only Portal is replaced. The new receipt
binds the immediately previous receipt, runtime, Portal image and deployment
timestamp. On activation failure or explicit rollback, restore that Portal with
the current environment and atomically restore its marker. Retained receipts
and runtimes are never modified by this release.

## Consequences

This is a bounded linear extension of ADR-0053, not an arbitrary cohort graph.
Old v1 releases keep their original retained rollback helper. New v2 receipts
require their own retained helper. Full-stack aliases, database state and all
other services remain outside this path. A foreign marker, baseline drift or
unselected-container change still requires reconciliation before deployment.
