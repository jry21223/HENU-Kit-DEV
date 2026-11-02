# Retain an independently released materials runtime during stack adoption

Production inspection after PR #630 confirmed that the application full alias
and materials control plane have different accepted versions. The full alias
is `11bf9e656b1253caddbd60afc9e77f5a07a812e4`; all nineteen installed materials
targets match `6008f76346fcc4dab1652505c371d62ebde3d30d`, including mode and owner.
That runtime was installed independently through the audited admin recovery
path. The old full payload has seventeen targets and lacks the later control
and resync protocol. Restoring it would remove delivered functionality.

## Decision

Allow the first stack transition to name one independent materials baseline
explicitly, with its full SHA, retained runtime and proof directory. Omission
keeps the existing full-baseline requirement. Do not infer a baseline from
matching individual live files or combine files from different runtimes.

Verify the retained archive and complete runtime against the existing v1 OIDC
manifest and bundle using the approved trust root, pinned repository, main ref,
workflow, source SHA and GitHub-hosted runner. Derive the historical run/attempt
from the verified certificate's invocation URI, verify that exact attempt
succeeded, and require the retained source to be an ancestor of the candidate.
Expired downloadable artifacts do not invalidate retained signed bytes.
Never execute the old installer or extend the image-signature contract.

Validate the complete materials payload and require its fixed target map to
match the candidate. Every live target must match that single baseline's
bytes, mode and root ownership. Bind the source/archive/proofs, payload manifest
and target inventory into the durable transition record and recheck them at
execution and recovery boundaries. Failure restores those actual before-images.
Recovery uses the recorded baseline and refuses conflicting new arguments.

Keep the application full baseline, scoped receipts, private credentials,
database routes, zero-migration boundary and all production gates unchanged.
This adopts the materials tool version only; it neither publishes materials nor
rewinds the active materials release, user databases or OSS objects.

For the retained application layer, use the full deployment timestamp already
stored in the root environment. Scoped Practice and Portal receipts keep their
own timestamps. The candidate configuration and receipt share one generated
timestamp. These metadata inputs do not relax the existing environment or
Compose hash checks.
