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

The first training session/attempt crosses ADR-0013's post-write boundary. Older
Core binaries such as c818 do not understand `purpose=training`; their ordinary
selectors and answer routes are unsafe against these new facts. Do not roll back
to them, or restore a pre-training backup and discard accepted work. Use forward
recovery (or a separately proven lossless reverse synchronization). The training
down migration refuses when any authored training unit or session exists. A data backup remains
necessary but is not permission to erase later accepted writes.

The release owner must reject an old incompatible rollback target after
`SELECT EXISTS(SELECT 1 FROM quizcraft_training_sessions)` is true, and select a
bundle that explicitly supports English training v1 across all three components.
This product ticket documents that requirement; it does not alter production
release policy or existing security configuration.

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
