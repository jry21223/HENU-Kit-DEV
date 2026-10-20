# H3C source preparation

The requested delivery is the nine teacher-provided H3C PDFs in Downloads,
their public HENU-Final-Review listing, a verified Library release, and usable
Practice banks with explanations. On 2026-10-20 the provider explicitly
confirmed the files may be public, and declined public attribution.

## Content contract

- Preserve source PDF bytes, names and SHA-256 in a source map. Keep nine
  independent original-document bank entries, categorized by GB0-192,
  GB0-330, GB0-372, GB0-382 and GB0-392, without claiming current official
  examination coverage. Cross-file duplicates retain their own source IDs.
- Extend the existing repository preparation tools with H3C source adapters;
  reuse `build_standard_bank_data`, existing analysis prompts, and stable
  question identity. Do not use a temporary parser or publish via the retired
  FastAPI management endpoints.
- Preserve every source question, its page range, original number and number
  segment. The GB0-372 main PDF restarts numbering after its first 20 items;
  those are separate source identities. An IPv4 address or command must never
  start a new question.
- Keep multiline commands and all option labels, including G/H. Convert
  reviewed answer labels to numeric indices before the existing A-F-oriented
  normalizer can discard a label. Treat textual fill answers as text.
- Extract source figures and retain their source coordinates. Render only
  content-addressed local PNG references in stems, options and explanations,
  preserving multiline commands. Support mobile containment and original-size
  viewing without nested interactive controls. Withhold ambiguous ownership or
  missing figures; a Markdown image token alone is not display acceptance.
- Missing answers/options, incomplete stems, source-answer conflicts and
  ambiguous boundaries enter an explicit review inventory. Preserve source
  evidence; do not guess missing facts. Source PDFs remain available as
  historical exercise material with visible review notes.
- Reuse meaningful source explanations after review. Generate missing
  explanations only for complete questions; flag suspected wrong answers
  instead of manufacturing a rationale. Record evidence-backed corrections
  separately. Reject empty, failed, generic or answer-inconsistent analyses.
- Deduplicate within one original document by question type, stem, ordered
  options and figure identity, retaining all provenance. Keep reordered
  variants separate because answer indices and explanation labels depend on
  option order. Do not deduplicate by source number. Preserve existing
  production bank and question IDs when updating known questions.

## Public test interfaces

The source conversion interface is PDF/text to source records plus a standard
candidate bank and review inventory. Focused red-green examples observe that
interface using independent source snippets: repeated numbering, IP addresses,
multiline options, blank answers, answer labels beyond F and missing figures.
The CLI must fail if its declared expected source count does not match.

The separately implemented Console-to-Workshop management interface must
preserve real Console authentication, the existing Workshop permissions,
dedicated signed service authentication, replay protection, complete version
readback, validation, publication, idempotency and expected-version checks.
Tests observe the actual HTTP boundary. The interface is disabled by default.

## Release and acceptance

Library publication and Practice publication are separate deliveries. Open
reviewable PRs, run the repository gates, and use the signed production release
process only after the exact changes are merged and approved for activation.
Use only the reviewed dedicated application-key procedure in ADR-0051; do not
create accounts/grants or signing keys, change IAM, use test JWTs, edit SQL
active versions, or bypass a failed gate. A Library merge or webhook 202 is not
acceptance: verify the active release and actual download bytes. Practice
acceptance requires full published-version readback, expected counts and a
real answer/analysis journey. Report withheld questions and incomplete stages.
