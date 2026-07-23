# PR Checklist

Use this checklist before submitting any pull request.

## Pre-PR

- [ ] One concern only (no mixing architecture + behavior + data + deployment)
- [ ] Branch named: `codex/<ticket-id>-<short-description>`
- [ ] Commit messages follow convention: `feat(scope): desc` or `[TICKET] desc`
- [ ] Dependencies on prior PRs identified and documented

## PR Body

- [ ] **What changed** — bullet list of concrete changes
- [ ] **Why** — rationale linked to spec/ticket
- [ ] **Impact** — scope boundary ("This PR does NOT...")
- [ ] **Affected modules** — checklist filled
- [ ] **Verification** — actual commands and output pasted
- [ ] **Closes #XX** — issue linked
- [ ] **Stack** — dependency PR linked if any

## Verification Gates

- [ ] Lint passes (no warnings)
- [ ] Type check passes (`tsc --noEmit` / `go vet`)
- [ ] Unit tests pass
- [ ] Contract tests pass (if API changed)
- [ ] Build succeeds
- [ ] Migration round-trip passes (if schema changed)

## Quality Checks

- [ ] No secrets, tokens, or credentials in code or logs
- [ ] No direct database access across service boundaries
- [ ] Product boundaries respected (no duplicate features)
- [ ] 360px mobile layout verified (if UI changed)
- [ ] `prefers-reduced-motion` respected (if animations added)
- [ ] Loading / Empty / Error / Success states complete (if UI added)

## Post-Merge

- [ ] CI green on main
- [ ] Dependent PRs rebased if needed
- [ ] Context docs updated if boundary changed
