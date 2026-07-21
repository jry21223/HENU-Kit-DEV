---
name: codex-workflow
description: >
  Structured GitHub-based development workflow for AI agents. Use when executing multi-PR feature work,
  planning implementation from a spec, creating architecture decision records, breaking work into issues,
  or following a disciplined branch-PR-merge-review cycle. Triggers on mentions of: implementation plan,
  spec execution, ADR, task breakdown, PR workflow, staged delivery, contract-first development,
  shadow/cutover migration, or when the user asks to "work like Codex" or follow a structured GitHub flow.
---

# Codex Workflow

A disciplined, GitHub-native development workflow for AI agents executing multi-step feature work.
Derived from analysis of 72 PRs across a 4-day sprint on a production monorepo.

## Core Principles

1. **Freeze decisions before code** — ADRs and context docs precede implementation
2. **Infrastructure before features** — unify tooling, split codebases, then build features
3. **Contract-first** — freeze API contracts before writing service code
4. **One PR, one concern** — never mix architecture, behavior, data, and deployment in one PR
5. **Full verification every time** — lint, test, build, contract checks in every PR
6. **Documentation as code** — ADRs, context glossaries, and PR templates are first-class artifacts

---

## The 7-Step Workflow

### Step 1: Freeze Decisions Before Code

Before writing any code, produce these artifacts:

**Architecture Decision Records (ADRs)** — one per major technical decision.
Use `templates/adr-template.md`. Number sequentially (0001, 0002, ...).

**Domain Context Glossary** — a `CONTEXT.md` at repo root defining canonical terms.
Each term needs: definition, scope boundary, and explicit "Avoid" synonyms.
Use `templates/context-template.md`.

**Executable Spec** — user stories with acceptance criteria and dependency graph.
Format: numbered tickets (e.g., HC-01 through HC-22), each with:
- What to build (1-3 sentences)
- Acceptance criteria (checkboxes)
- Dependencies (blocked-by list)
- Out of scope (explicit exclusions)

**Context Map** — a `CONTEXT-MAP.md` indexing all domain contexts with relationships.

### Step 2: Configure Agent Infrastructure

**AGENTS.md** at repo root — skill routing and domain doc discovery protocol:
```markdown
# Repository agent guidance
## Domain docs
This is a multi-context Monorepo. Start from CONTEXT-MAP.md,
then read relevant context glossaries and ADRs.
```

**Triage Labels** — five canonical states for issue lifecycle:
- `needs-triage` — new, not yet categorized
- `needs-info` — blocked on clarification
- `ready-for-agent` — clear enough for AI execution
- `ready-for-human` — needs human review or decision
- `wontfix` — intentionally not addressed

**PR Template** — enforce security, accessibility, product boundary, and rollback checks.
Use `templates/pr-template.md`.

### Step 3: Infrastructure Before Features

Execute infrastructure work as its own PRs, each independently verifiable and reversible:

1. **Package manager unification** (e.g., npm → pnpm migration)
2. **Code physical separation** (e.g., split monolith into independent apps)
3. **CI/CD setup** (path-filtered workflows per service)
4. **Repository agent configuration** (AGENTS.md, labels, templates)

Each infrastructure PR should:
- Have a clear, scoped title: `build: migrate frontend workspace to pnpm`
- Include verification commands in the PR body
- Be mergeable independently of feature work

### Step 4: Contract-First Vertical Slices

For each service/feature slice:

1. **Freeze the OpenAPI contract** first (PR N)
2. **Generate typed clients** from the contract (Go, TypeScript)
3. **Implement the service** with schema migration + handler + tests + Docker build (PR N+1)
4. **CI verifies**: contract drift, breaking changes, lint, vet, test, build

A vertical slice is self-contained: it owns its schema, business logic, HTTP layer, tests, and deployment config. Never split a single service's concerns across multiple PRs unless one is clearly a dependency of the other.

### Step 5: Progressive Integration (Shadow/Cutover Pattern)

For migrations or replacements of existing systems:

1. **Shadow reads** — new service reads alongside old, responses compared
2. **Reconciliation** — verify data consistency between old and new
3. **Gradual cutover** — shift read traffic incrementally, then writes
4. **Rollback snapshot** — verified backup before any production change
5. **Read-only observation** — old system stays read-only for at least one release cycle
6. **Targeted fixes** — post-cutover PRs for edge cases discovered in production

### Step 6: One PR, One Concern, Full Verification

Every PR must include:

```markdown
## What changed
- Bullet list of concrete changes

## Why
- Rationale linked to spec/ticket

## Impact
- Scope boundary: "This PR does NOT..."
- Affected modules checklist

## Verification
- Actual commands and their output
- Lint, test, build, contract checks
- Independent review result

## Closes #XX
## Stack: Base: #YY (dependency)
```

**Gate**: `Standards 0 findings; Spec 0 findings` — no PR merges with unresolved review findings.

**Commit message format**:
- Conventional commits: `feat(scope): description` or `fix(scope): description`
- Or bracketed ticket refs: `[HC-15] deliver Food operations`
- Merge commits: standard GitHub `Merge pull request #N from branch`

### Step 7: Documentation as Code

Maintain these artifacts throughout the project:

| Artifact | Purpose | Location |
|----------|---------|----------|
| ADRs | Architectural decisions | `docs/adr/NNNN-title.md` |
| Context Map | Domain term index | `CONTEXT-MAP.md` |
| Context Glossaries | Per-service terms | `apps/*/CONTEXT.md`, `services/*/CONTEXT.md` |
| PR Template | Enforced checks | `.github/pull_request_template.md` |
| Implementation Plan | Dependency graph | `docs/development/implementation-plan.md` |
| Go/No-Go Checklist | Hard stop conditions | `docs/development/go-no-go-checklist.md` |
| CI Workflows | Path-filtered gates | `.github/workflows/*.yml` |

---

## Branch Naming Convention

```
codex/<ticket-id>-<short-description>
```

Examples:
- `codex/hc-01-architecture-baseline`
- `codex/hc-05-platform-core-session`
- `codex/hc-19-quizcraft-ranking`

For non-ticketed work: `codex/<short-description>` (e.g., `codex/repo-agent-config`)

---

## Task Decomposition Strategy

Break work into numbered tickets following dependency order:

1. **Documentation first** — freeze architecture before any code
2. **Infrastructure second** — tooling, physical split, CI
3. **Foundation third** — base shells, shared services
4. **Services fourth** — implement service slices (contract → implementation)
5. **Integration fifth** — wire services together (gateway, overview)
6. **Migration sixth** — shadow reads, reconciliation, cutover
7. **Fixes last** — targeted PRs for production edge cases

Each ticket = exactly one PR = exactly one concern.

---

## CI/CD Patterns

Path-filtered workflows per service:

```yaml
on:
  pull_request:
    paths:
      - 'services/platform-core/**'
      - '.github/workflows/platform-core.yml'
```

Standard CI gates per service:
- **Go**: `go vet`, `staticcheck`, `govulncheck`, `go test -race`, migration round-trip
- **TypeScript/Vue**: `tsc --noEmit`, `vitest run`, `playwright test`
- **Contracts**: OpenAPI lint (Redocly), breaking-change detection (oasdiff), generated-client drift check
- **Docker**: build verification, Trivy secret/vulnerability scan
- **Migration**: up-all → down-all → up-all round-trip, pg_dump/restore recovery

---

## Quick Reference

When the user asks you to execute a spec or implement a multi-PR feature:

1. Read the spec and existing context docs
2. Decompose into numbered tickets with dependencies
3. Create ADRs for major decisions
4. Set up AGENTS.md and PR template if not present
5. Execute tickets in dependency order, one PR each
6. Every PR: verify, review, merge before starting next
7. Document decisions as you go

Read `templates/pr-template.md`, `templates/adr-template.md`, and `templates/context-template.md` for ready-to-use formats.
