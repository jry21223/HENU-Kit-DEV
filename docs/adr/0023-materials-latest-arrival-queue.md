---
status: accepted
amends: 0022
---

# Materials candidate preparation uses a latest-arrival, unprivileged queue

Candidate preparation needs a durable receiver/consumer boundary before a
later activation slice can consider publishing it. The generic HENU Kit deploy
queue is FIFO and drives a different release contract, so changing it to
coalesce deliveries would change unrelated deployments.

## Decision

- Materials uses an instance-scoped queue policy. Under its existing state
  lock it holds zero or one running delivery and zero or one waiting delivery.
  A newly accepted materials delivery replaces the waiting delivery, if any;
  it never interrupts the running preparation.
- "Latest" means the last delivery accepted by the receiver, in arrival order.
  It does not infer Git topology, compare commit dates, or replace the fixed
  SHA/ref validation required by ADR-0022.
- A replaced delivery is retained as a terminal duplicate marker for the
  configured retention window. A redelivery of it cannot displace the newest
  waiting delivery. After an interrupted run, recovery requeues the interrupted
  delivery only when no newer waiting delivery exists.
- The generic queue keeps its FIFO constructor, commands, retry behavior, and
  public API. Materials coalescing is selected only by explicit materials
  receiver and runner commands.
- The materials receiver and candidate-preparation consumer run as the same
  unprivileged `henukit-deploy` account. A fixed wrapper obtains its source
  repository, allowed ref, and candidate root from operator configuration; the
  signed queue event cannot choose a command, filesystem path, public root, or
  database target.

## Consequences

- Queueing a candidate does not activate it. This decision does not enable a
  service, create a root runtime process, publish a public tree, or import the
  Study catalog.
- The wrapper invokes only the ADR-0022 candidate-preparation boundary. It does
  not invoke the legacy synchronizer, Docker, psql, or a database migration.
- Installing a materials receiver into the fixed-artifact production delivery
  path remains a later reviewed activation decision. Repository unit templates
  are not evidence that a host service is enabled.
