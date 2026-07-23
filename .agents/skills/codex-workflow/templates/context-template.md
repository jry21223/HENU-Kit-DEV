# [Service/App Name] Context

## Owns

- [List what this service/app owns: data, business logic, UI surfaces]
- [Be specific: "user sessions" not "auth stuff"]

## Does not own

- [List what explicitly belongs to other services]
- [Include the owning service name: "Account database (owned by Platform Core)"]

## Current boundary

[1-3 paragraphs describing the current implementation state.
What has been built? What is the next milestone?
Include specific PR/ticket references if available.]

## Key terms

- **[Term 1]**: [Definition]. [Scope boundary — what it includes and excludes].
- **[Term 2]**: [Definition]. Avoid: [synonym 1], [synonym 2].
- **[Term 3]**: [Definition]. [Relationship to other terms].

[Each term should have:
1. A clear definition (1-2 sentences)
2. A scope boundary (what it includes/excludes) OR
3. Explicit "Avoid" synonyms to prevent terminology drift]

## Tech stack

[Framework, language, key libraries, database, cache layer.
Be specific with versions if they matter for compatibility.]

## Relationships

- **[Service A] → [This service]**: [How they interact, protocol, data flow]
- **[This service] → [Service B]**: [How they interact, protocol, data flow]
