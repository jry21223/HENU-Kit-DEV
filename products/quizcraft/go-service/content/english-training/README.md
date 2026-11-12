# Listening publication package

`north-wind.import.json` is an explicit Workshop import payload, not a startup seed.
Create a new empty bank through the existing owner lifecycle, substitute the bank's
current `expected_version`, then import, inspect its resolved owner version, validate,
and publish using the existing permissions and optimistic lifecycle version.
Never use direct SQL to activate this package. Production publication is pending.

The audio is P. Roach / International Phonetic Association (2003), CC BY-SA 3.0;
the Wikimedia Commons captions and HENU Kit adaptation/translation are CC BY-SA 4.0.
Component-specific source, license, attribution and modifications are in the payload.
The four comprehension questions are original HENU Kit practice; no official CET
provenance is claimed. The payload uses the frozen review categories: q2 is detail,
q3 is logic and q4 is inference, retaining the frozen bounded category vocabulary.

The checked immutable MP3 in `../../media/` is 584180 bytes, SHA256
`92aa98d9bf83c7f1532dcd2ae2169190b649b5214c13481405ce68d14a630336`.
Its gapless MPEG timing is 36.426848 seconds (rounded authoring duration: 36427ms).
Sentence timestamps retain the human-edited Commons SRT boundaries. No evenly
spaced or ASR-invented timing is used. English/Chinese text and keys remain in this
owner-only payload and database, never in a public Portal bundle.

The source editorial history, source-text semantic review, offline ASR comparison,
and automated real-media decode are recorded in the research evidence. A new human
ear review of all five sentence ranges and answer evidence remains **pending**.
Automated test publication is isolated acceptance only and does not satisfy that
human gate or claim production publication. Retain that distinction in operator notes.

## Deployment and recovery boundary

Release the compatible Core/Gateway/Portal bundle and migration 000012, verify
health and default gates, then explicitly import/validate/publish this unit, and
only then permit the first learner training write. Publication by itself does not
prove human listening acceptance; that gate must be recorded separately.

The compatibility boundary begins with **any authored training content**, including
an unpublished imported unit before a learner session or attempt exists. Older
Core binaries such as c818 do not understand training purpose or bindings; their
ordinary selectors, answer routes and statistical consumers are unsafe once these
facts exist. Never restore a pre-training backup to discard accepted work. Use a
verified compatible immediately previous bundle or forward recovery. Migration
000012 down refuses authored training units or sessions; backups remain recovery
evidence, not permission to erase later writes.

The same full release owner uses the Core-owned read-only release-state probe,
which recognizes only exact healthy tracked schemas11/12 and includes authored
units/versions/members, training-purpose/bound sessions, outcomes and disclosures.
A session-only SQL existence check cannot establish compatibility. Once the probe
requires `english-training-data-v1`, all three source-bound Core/Gateway/Portal
consumers in the original immediate-previous target must be verified before
replacement. Unknown state or missing/tampered evidence holds maintenance. See
[the schema release procedure](../../../../../docs/operations/henukit-quizcraft-schema-release.md).
Initial healthy runtime/schema activation must precede this normal Console import,
validate and publish journey. No release step pre-seeds or auto-publishes the unit.

Migration 000012 adds six small tables, one default-ordinary session column,
immutable/deferred consistency guards and three projection views; it does not
rewrite historical attempts or settlements. PostgreSQL takes an ALTER TABLE lock
on sessions and creates new empty relations; schedule schema expansion through the
release owner's normal migration window. The training data volume starts at zero.
Use the versioned migrator, verify its receipt/checksum, and check
`quizcraft_training_sessions`, `quizcraft_training_unit_versions` and the ordinary
purpose default before activation. Before any authored training content exists, down/reapply
is tested with ordinary content retained. Once a training unit is imported, down fails closed
and the forward-recovery rule above applies.
