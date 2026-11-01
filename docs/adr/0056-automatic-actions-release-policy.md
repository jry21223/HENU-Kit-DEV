# Opt-in automatic full releases from successful main builds

The operator explicitly requested automatic full application deployment after
the complete main release workflow succeeds. This changes who initiates a
routine release; it does not waive any production technical gate.

Use a separate root scheduler with a private, root-owned policy pinned to the
repository, main branch and release workflow. A nonprivileged HMAC-verified
workflow notification only wakes that scheduler. GitHub API lookups independently
select and recheck the newest successful main push run. Missed notifications
are reconciled by polling.

The scheduler invokes the existing full activation command, including its
blocker, EasyPay, verified artifacts, database restore tests, exact-SHA approval,
health and rollback requirements. It never writes approval files itself.
It records and synchronizes one immutable automatic-attempt audit before invoking
the command. A failed or interrupted SHA is not retried automatically, including
after a workflow rerun; an operator must reconcile it or publish a new SHA.
Its run/attempt fields record scheduling intent, not new artifact provenance.
The existing SHA-keyed verified artifact cache remains unchanged.

Only one scheduler may be active. The existing watcher must first drain through
its safe quiescence protocol. Automatic activation also uses the existing direct
deployment lock. Shutdown signals request a stop after the active release has
finished; they do not kill its activation or rollback children.

Existing scoped markers and pending rollback contracts continue to block
automatic full activation. This ADR does not invent a mixed-baseline adoption,
degraded recovery or configuration override. In particular, the current Portal
membership checkpoint and Practice private credentials require a separately
reviewed full-release transition; deleting their marker is prohibited.

The notification state is a bounded, atomically replaced hint with recent
delivery deduplication. It is separate from both the retired source deployment
FIFO and the materials queue, and never claims that notification delivery is a
successful application deployment. Enabling this path publishes no materials.
