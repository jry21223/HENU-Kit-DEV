# Portal releases after an active membership checkpoint

The production scoped marker now identifies the active Account Portfolio/Nginx
membership release. Its immutable predecessor is the retained Portal chain.
Rewriting that marker or rolling back membership to publish a frontend fix
would misrepresent or remove delivered functionality.

## Decision

Extend only the Portal successor entry to accept an active
`henukit-account-membership-v1` checkpoint over a retained Portal predecessor.
Verify its canonical private receipt, runtime/helper/Compose hashes, current
environment and backup, exact private configuration inventory, original Nginx
source, two selected container snapshots, image identities, effective
environments, configuration hashes and read-only mount through the retained
hash-bound helper's read-only configuration check. All other container facts
must still match the historical Portal receipt.

A `henukit-portal-successor-v3` receipt binds that membership checkpoint by
path/digest separately from the actual previous Portal runtime/image/receipt.
Consecutive Portal releases retain the same checkpoint and linear Portal
history; count the checkpoint within the existing 64-receipt limit. Only
Portal is replaced. Existing source, provenance, watcher, disk, environment,
configuration and rollback gates remain required.

On rollback restore the immediately previous Portal and preserve membership
and all other containers. Since restoring Portal recreates its container ID,
retain the new `rolled_back` receipt/marker and its verified Portal checkpoint;
never restore a membership marker whose old unselected snapshot would falsely
claim the original Portal ID. Repeated rollback verifies this final checkpoint.
An interrupted or rolled-back checkpoint blocks fresh activation pending a
separately reviewed reconciliation. Pre-switch rollback may leave the original
marker only while the original Portal snapshot still matches.

## Boundaries

This supports one active membership checkpoint and its subsequent Portal
releases. Unknown scopes, inactive membership records, another membership
transition, altered history/configuration and container drift fail closed.
It does not change membership runtime, Nginx, credentials, database state,
full-stack aliases, or deferred Career/getWork acceptance.
