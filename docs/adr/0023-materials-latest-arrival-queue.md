---
status: accepted
amends: 0022
superseded_by: 0025
---

# Materials candidate preparation uses a latest-arrival, unprivileged queue

Candidate preparation needs a durable receiver/consumer boundary before the
ADR-0025 activation path can publish it. The generic HENU Kit deploy
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
- The materials receiver and the A01 candidate-preparation subprocess run as
  the unprivileged `henukit-deploy` account. ADR-0025 moves the queue consumer
  to a confined root runner so the same accepted event can cross the sealed
  activation boundary. A fixed preparation wrapper obtains its source
  repository, allowed ref, and candidate root from operator configuration; the
  signed queue event cannot choose a command, filesystem path, public root, or
  database target.

## Consequences

- Queueing a candidate does not activate it. ADR-0025 separately owns the root
  runtime, public-tree publication, and Study catalog import.
- The wrapper invokes only the ADR-0022 candidate-preparation boundary. It does
  not invoke the legacy synchronizer, Docker, psql, or a database migration.
- ADR-0025 defines the reviewed activation decision. Repository unit templates
  and installer output still are not evidence that a production host is enabled.
