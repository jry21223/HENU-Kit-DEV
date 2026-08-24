import assert from "node:assert/strict";
import test from "node:test";

import {
  assertPublicRouteNavigation,
  assertSerializedDOMNoSecrets,
} from "./oauth-continuation-journey.mjs";

test("serialized DOM leakage checks include hidden attributes and script data", () => {
  const state = "state_portal_hidden_01";
  const challenge = "challenge_hidden_01";

  assert.throws(
    () =>
      assertSerializedDOMNoSecrets(
        `<html><body><input type="hidden" data-oauth-state="${state}"></body></html>`,
        [state],
      ),
    /serialized DOM contains an OAuth continuation secret/,
  );
  assert.throws(
    () =>
      assertSerializedDOMNoSecrets(
        `<html><body><script>window.__BOOTSTRAP__={challenge:"${challenge}"}</script></body></html>`,
        [challenge],
      ),
    /serialized DOM contains an OAuth continuation secret/,
  );
  assert.doesNotThrow(() =>
    assertSerializedDOMNoSecrets("<html><body><p>安全恢复</p></body></html>", [
      state,
      challenge,
    ]),
  );
});

test("only exact Next route hydration may repeat the current continuation handle", () => {
  const handle = "continuation_handle_bound_to_this_browser_01";
  const exactHydration = [
    String.raw`\"c\":[\"\",\"account\",\"login?continuation=${handle}\"],\"q\":\"?continuation=${handle}\"`,
    String.raw`\"children\":[\"__PAGE__?{\\\"continuation\\\":\\\"${handle}\\\"}\"`,
    String.raw`\"serverProvidedParams\":{\"searchParams\":{\"continuation\":\"${handle}\"},\"params\":{},\"promises\":null}`,
  ].join("");

  assert.doesNotThrow(() =>
    assertSerializedDOMNoSecrets(exactHydration, [handle], {
      currentContinuation: handle,
    }),
  );
  for (const leak of [
    `<input type="hidden" data-continuation="${handle}">`,
    `<script>window.untrustedContinuation="${handle}"</script>`,
  ]) {
    assert.throws(
      () =>
        assertSerializedDOMNoSecrets(`${exactHydration}${leak}`, [handle], {
          currentContinuation: handle,
        }),
      /serialized DOM contains an OAuth continuation secret/,
    );
  }
});

test("public route navigation rejects errors and unexpected final URLs", () => {
  assert.throws(
    () =>
      assertPublicRouteNavigation({
        status: 404,
        finalURL: "https://portal.example/missing",
        expectedURL: "https://portal.example/missing",
      }),
    /status 404/,
  );
  assert.throws(
    () =>
      assertPublicRouteNavigation({
        status: 200,
        finalURL: "https://portal.example/account/login",
        expectedURL: "https://portal.example/practice/stats",
      }),
    /unexpected URL/,
  );
  assert.doesNotThrow(() =>
    assertPublicRouteNavigation({
      status: 200,
      finalURL: "https://portal.example/practice/stats",
      expectedURL: "https://portal.example/practice/stats",
    }),
  );
});
