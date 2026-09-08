# Account Center continuation acceptance

## Release gate

Run the cumulative gate from the repository root:

```bash
pnpm run test:oauth-continuation
```

The command must execute the same parameterized browser journey for Portal and
Console. It is a required dependency of both the fixed-SHA image build and the
runtime package job. A healthy endpoint, a page `200`, or a single redirect is
not acceptance.

Every artifact path consumes a canonical receipt containing the release SHA,
its Git tree, and a passing result. GitHub Actions uploads that receipt from the
gate job and every image/runtime job verifies it before construction. The
formal WSL builder and the quick local builder run the same gate themselves;
the runtime packager rejects direct calls without the matching receipt.

The gate must prove all of the following:

- a signed-out browser reaches the Portal-owned Account Center, completes a
  fresh authentication, crosses the exact callback, receives the product
  Session, and returns to the original safe path;
- an existing Core Session uses the authorize fast path without reopening the
  Account Center;
- expired, replayed, browser-mismatched, tampered, unknown, unsupported, and
  callback-mismatched requests fail closed;
- desktop and 360px reduced-motion journeys remain keyboard usable;
- final URLs, rendered DOM, external requests, and Referer headers do not leak
  the authorize query, OAuth state, PKCE challenge, Authorization Code,
  continuation handle, or internal callback;
- structured events contain exactly a request ID, trusted client ID, outcome
  category, and duration. Never retain the raw query, continuation, credential,
  code, or callback.

## Account Center transport invariant

The browser is expected to make one initial transport request to
`/account/login?continuation=...`. The Portal proxy must answer with `303`,
`Cache-Control: private, no-store, max-age=0`, and
`Referrer-Policy: no-referrer`; that response must not contain Account Center
HTML. Its same-origin Location moves the handle into a URL fragment. The
following document request, all eager fonts and scripts, and every later
Referer must use the clean `/account/login` URL.

The client reads and removes the fragment before paint, then retains the handle
only in component state. It deliberately calls the native
`History.prototype.replaceState`, not Next's patched history instance: the
patched method can start an RSC request while the old URL is still the document
referrer. A transient bootstrap retry must reuse the in-memory handle without
restoring it to either the query or fragment. Any change to this sequence must
rerun both product journeys and preserve the strict font, script, fetch, form,
callback, URL, DOM, and Referer assertions.

## Production acceptance

Use a new private browser context for every production journey. Do not reuse a
state, Authorization Code, continuation handle, or product Session from a prior
attempt. Exercise Portal and Console separately, including one 360px keyboard
journey, and record the safe destination and user-visible recovery text. Do not
copy credential values or complete OAuth URLs into evidence.

Record these facts as separate claims:

1. CI run and candidate commit SHA.
2. Merged main SHA.
3. Deployed release SHA reported by the production runtime.
4. Fresh production user-journey evidence for Portal and Console.
5. Public-copy scan results for Portal, Console, and the practice pages.

If any SHA differs, or only health checks are available, the release remains
unverified. Keep the implementation Issue open until the deployed SHA and all
fresh production journeys are recorded.
