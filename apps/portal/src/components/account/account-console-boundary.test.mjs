import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const accountConsole = new URL("../../app/account/(console)/", import.meta.url);

async function readConsoleSource(name) {
  return readFile(new URL(name, accountConsole), "utf8");
}

test("account console keeps browser identity on the real Portal Session boundary", async () => {
  const [layout, overview, wallet, membership, notifications, tickets, security, sessionBoundary] = await Promise.all([
    readConsoleSource("layout.tsx"),
    readConsoleSource("page.tsx"),
    readConsoleSource("wallet/page.tsx"),
    readConsoleSource("membership/page.tsx"),
    readConsoleSource("notifications/page.tsx"),
    readConsoleSource("tickets/page.tsx"),
    readConsoleSource("security/page.tsx"),
    readFile(new URL("./account-console-session.tsx", import.meta.url), "utf8"),
  ]);

  for (const source of [layout, overview, wallet, membership, notifications, tickets, security, sessionBoundary]) {
    assert.doesNotMatch(source, /@\/lib\/auth\/store|\bauthStore\b/);
    assert.doesNotMatch(source, /\blocalStorage\b|\bsessionStorage\b/);
    assert.doesNotMatch(source, /\bmockAllowed\b|\bisMockAuthEnabled\b/);
    assert.doesNotMatch(source, /@\/lib\/auth\/mock|\baccountStore\b/);
  }

  assert.match(layout, /fetchSession/);
  assert.match(layout, /AccountConsoleSessionProvider/);
  assert.match(layout, /requireLogin/);
  assert.doesNotMatch(layout, /\bAccountEntry\b/);
  assert.match(overview, /useAccountConsoleSession/);
  for (const source of [overview, wallet, membership, notifications, tickets]) {
    assert.match(source, /useAccountConsoleUnauthorizedHandler/);
  }
});

test("account navigation exposes only delivered Account Portfolio capabilities", async () => {
  const layout = await readConsoleSource("layout.tsx");

  assert.match(layout, /href: "\/account\/membership"/);
  assert.doesNotMatch(layout, /href: "\/account\/(posts|deals)"/);
});

test("legacy auth helpers retain no account dashboard fixtures", async () => {
  const authMock = await readFile(new URL("../../lib/auth/mock.ts", import.meta.url), "utf8");

  assert.doesNotMatch(
    authMock,
    /\b(accountStore|AccountData|MEMBERSHIP_PLANS|FREE_MEMBERSHIP|TicketMsg|unreadNotices)\b/
  );
});
