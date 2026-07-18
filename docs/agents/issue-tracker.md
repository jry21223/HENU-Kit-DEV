# Issue tracker: GitHub

Issues and PRDs for this repo live as GitHub Issues in `jry21223/HENU-Kit-DEV`. Use the `gh` CLI for all operations.

## Conventions

- **Create an issue**: `gh issue create --title "..." --body "..."`. Use a temporary body file for long Markdown bodies on Windows.
- **Read an issue**: `gh issue view <number> --comments`, including labels.
- **List issues**: `gh issue list --state open --json number,title,body,labels,comments` with appropriate label and state filters.
- **Comment on an issue**: `gh issue comment <number> --body "..."`.
- **Apply or remove labels**: `gh issue edit <number> --add-label "..."` / `--remove-label "..."`.
- **Close**: `gh issue close <number> --comment "..."`.

Infer the repository from `git remote -v`; `gh` does this automatically inside this clone.

## Pull requests as a triage surface

**PRs as a request surface: no.**

GitHub shares one number space across issues and PRs. Resolve an ambiguous `#42` with `gh pr view 42` and fall back to `gh issue view 42`.

## Skill operations

- When a skill says **publish to the issue tracker**, create a GitHub Issue.
- When a skill says **fetch the relevant ticket**, run `gh issue view <number> --comments`.
- Specifications may live in an issue body or a linked repository Markdown document; the issue must identify the authoritative source.

## Dependencies and maps

- A work map is one issue labelled `wayfinder:map`; child tickets are GitHub sub-issues where supported.
- Use GitHub native issue dependencies as the canonical blocking representation.
- When native sub-issues or dependencies are unavailable, add `Part of #<map>` and `Blocked by: #<issue>` lines to child issue bodies.
- A ticket is ready only when every blocker is closed and its acceptance criteria are complete.
- Claim work with `gh issue edit <number> --add-assignee @me` before implementation.
- Resolve work by posting verification evidence, closing the issue, and updating its parent map or dependency chain.
