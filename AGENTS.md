# Repository agent guidance

## Agent skills

### Engineering workflow (mandatory): Ask Matt first, always

**Every engineering task starts by consulting the `ask-matt` router
(`~/.agents/skills/ask-matt/SKILL.md`) and follows the flow it routes to.**
Do not improvise an alternative workflow. The canonical flow:

- **Main flow: idea → ship**
  1. `/grill-with-docs` — sharpen the idea by interview (stateful: retains what
     it learns in `CONTEXT.md` + ADRs). Start here when we have a codebase.
  2. Branch: a question needs a runnable answer (state, business logic, UI)?
     Detour through `/prototype`, bridged by `/handoff` in both directions.
  3. Branch: multi-session build?
     - Yes → `/to-spec`, then `/to-tickets` (each ticket declares blocking
       edges), then `/implement` per ticket, clearing context between tickets,
       working blockers-first.
     - No → `/implement` in the current context.
- **`/implement` always drives `/tdd` internally** — one red-green slice at a
  time — and closes out with **`/code-review`** (Standards + Spec two-axis
  review of the diff, plus the Public-ready Copy third axis below) **before
  committing**. Reach for `/tdd` alone for a concrete behaviour test-first
  without a full spec; `/code-review` alone to review any branch or PR against
  a fixed point.
- **On-ramps**: incoming bugs/requests → `/triage` (only for issues we did not
  create; never triage tickets `/to-tickets` produced). Hard bugs → `/diagnosing-bugs`
  (tight feedback loop first, regression test after). Huge/foggy efforts →
  `/wayfinder` (produces decisions, hands off to `/to-spec` — never loop
  straight into `/implement` unless the effort is genuinely small).
- **Codebase health**: run `/improve-codebase-architecture` opportunistically.
- **Context hygiene**: keep grilling → spec → tickets in one unbroken context
  window; each `/implement` starts fresh from its ticket. Stay inside the smart
  zone; if a session approaches it before `/to-tickets`, `/handoff` to a fresh
  thread instead of pushing on degraded. `/handoff` forks (new session);
  `/compact` continues (same session) — use `/compact` only at intentional
  phase breaks, never mid-phase.
- **Vocabulary underneath**: `/domain-modeling` for domain language and ADRs;
  `/codebase-design` for module shape.
- **Precondition**: `/setup-matt-pocock-skills` before the first flow (tracker,
  triage labels, doc layout).

### Review prompt baseline

For every Standards / Spec review, also inspect added or changed user-visible copy for **Public-ready Copy**. Report only evidence-backed findings: copy must not expose placeholders, test/debug/internal details, unsupported claims, or unintended visible environment URLs/accounts; errors must tell the user what happened and what they can do; wording must fit the product's tone. Report this as a third axis, not a substitute for Standards or Spec. If the diff has no user-visible copy, explicitly report `Public-ready Copy: not applicable`.

### Issue tracker

Issues and PRDs are tracked in GitHub Issues for `jry21223/HENU-Kit-DEV`. See `docs/agents/issue-tracker.md`.

### Triage labels

Use the canonical labels `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, and `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

This is a multi-context Monorepo. Start from `CONTEXT-MAP.md`, then read relevant context glossaries and ADRs. See `docs/agents/domain.md`.
