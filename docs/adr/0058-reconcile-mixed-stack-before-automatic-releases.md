# Establish a real full baseline before automatic releases

The operator selected automatic whole-stack releases after complete main builds.
Production retains a full alias plus independently released Practice, membership
and Portal services. The active Portal v3 receipt binds those checkpoints;
clearing its marker would invent a full rollback baseline that never existed.

## Decision

Add one explicit stack-transition entry, separate from the routine scheduler.
It accepts the verified active v3 chain, reconstructs the real mixed Compose
configuration and retains original receipts and private configuration authority.
The fixed cohort includes every application service and Nginx; persistent
PostgreSQL/Redis and the disabled crawler/relay remain outside it.

Require current-main/newest-run identity, all executed runtime bytes bound to
the attested archive, unchanged owner migrations and data routes, and the
existing full production technical gates. The transition may verify existing
operator grants and EasyPay deployment but does not grant new permissions,
install gateway patches or migrate owner schemas. WSL availability is replaced
only by the explicit disabled Career source contract in ADR-0057.

Use existing deployment/watcher locks, safe quiescence, fresh owner backups and
isolated restore proofs. Preserve private mixed/candidate configurations and
durable transaction phases. Install the actual verified materials runtime while
its automatic trigger is quiesced; use its real before-images and orchestration
lock for recovery. Reject legacy materials state requiring another migration.

Only after full image/configuration, public-route, authentication and disabled
source acceptance may both full aliases and the last-activated SHA advance and
the owned scoped marker move into the transition audit. A durable accepted
boundary precedes restoring the original materials trigger; legitimate queued
work starting afterward cannot cause an application rollback.

Before acceptance, failure restores the actual mixed images, private environment,
configuration, stopped states, materials runtime and aliases/marker. Recovery
refuses foreign container or configuration drift, never rewrites historical
receipts and never overwrites production user databases with old backups.
After acceptance, recovery completes the control plane instead of undoing
business state already exposed to queued work. Routine scheduling remains
disabled until this transition has been verified on production.

See [the operational procedure](../operations/henukit-stack-transition.md).
