import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import {
  chmodSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const checker = fileURLToPath(
  new URL("../check-account-production-boundary.mjs", import.meta.url),
);
const releaseSha = "a".repeat(40);

function write(root, path, contents) {
  const destination = join(root, path);
  mkdirSync(dirname(destination), { recursive: true });
  writeFileSync(destination, contents);
}

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "account-production-boundary-"));
  write(
    root,
    "apps/portal/src/app/account/(console)/layout.tsx",
    'import { fetchSession } from "@/lib/api/client";\nexport default fetchSession;\n',
  );
  write(
    root,
    "apps/portal/src/components/account/account-console-session.tsx",
    'export const boundary = "portal-session";\n',
  );
  write(
    root,
    "apps/portal/src/lib/auth/mock.ts",
    'export const EMAIL_DEMO_CODE = "local-only";\n',
  );
  write(root, "apps/portal/src/lib/api/client.ts", "export const fetchSession = async () => ({});\n");
  write(root, "services/portal-gateway/internal/accountportfolio/client.go", "package accountportfolio\n");
  write(root, "services/console-gateway/internal/accountportfolio/client.go", "package accountportfolio\n");
  write(
    root,
    "apps/portal/Dockerfile",
    "ARG NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=1\nENV NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=$NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY\n",
  );
  write(
    root,
    "docker-compose.henukit.yml",
    "services:\n  portal-api:\n    environment:\n      PORTAL_API_MODE: ${PORTAL_API_MODE:-live}\n",
  );
  write(
    root,
    ".github/workflows/deploy-henukit.yml",
    "run: scripts/ops/henukit-release-images.sh --github-matrix\n",
  );
  const inventory = join(root, "scripts/ops/henukit-release-images.sh");
  write(
    root,
    "scripts/ops/henukit-release-images.sh",
    "#!/usr/bin/env bash\ncase \"$*\" in\n  --check) exit 0 ;;\n  '--field portal build_args') printf 'NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=1\\n' ;;\n  *) exit 64 ;;\nesac\n",
  );
  chmodSync(inventory, 0o755);
  write(
    root,
    "services/account-portfolio/cmd/server/main.go",
    'func paymentProviderFromEnv() PaymentProvider {\n if os.Getenv("ACCOUNT_PORTFOLIO_EASYPAY_ENABLED") != "1" { return nil }\n return NewEasyPayProvider()\n}\n',
  );
  return root;
}

test("production boundary emits an exact-SHA pass manifest for a real-gateway account surface", () => {
  const root = fixture();
  const report = join(root, "account-production-boundary.env");

  execFileSync(process.execPath, [checker, "--repo-root", root, "--report", report], {
    env: { ...process.env, RELEASE_SHA: releaseSha },
  });

  assert.equal(
    readFileSync(report, "utf8"),
    [
      `release_sha=${releaseSha}`,
      "status=pass",
      "account_console_mock_sources=absent",
      "account_transitive_mock_sources=absent",
      "account_payment_provider=easypay_or_disabled",
      "portal_require_gateway=1",
      "portal_allow_mock=0",
      "portal_api_default_mode=live",
      "",
    ].join("\n"),
  );
});

test("production boundary rejects a runtime-wired fake payment provider", () => {
  const root = fixture();
  write(
    root,
    "services/account-portfolio/cmd/server/main.go",
    "func paymentProviderFromEnv() PaymentProvider { return NewFakePaymentProvider() }\n",
  );

  const result = spawnSync(process.execPath, [checker, "--repo-root", root], {
    encoding: "utf8",
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /runtime.*fake payment provider/i);
});

test("production boundary rejects a mock reached through an indirect Account import", () => {
  const root = fixture();
  write(root, "apps/portal/src/lib/api/client.ts", 'export { accountData } from "@/lib/api/account";\n');
  write(root, "apps/portal/src/lib/api/account.ts", 'export { accountData } from "@/lib/account/fixture";\n');
  write(root, "apps/portal/src/lib/account/fixture.ts", "export const accountData = {};\n");

  const result = spawnSync(process.execPath, [checker, "--repo-root", root], {
    encoding: "utf8",
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /reachable mock or fixture code/i);
});

test("production boundary rejects an Account console import from a mock data source", () => {
  const root = fixture();
  write(
    root,
    "apps/portal/src/app/account/(console)/wallet/page.tsx",
    'import { accountStore } from "@/lib/auth/mock";\nexport default accountStore;\n',
  );

  const result = spawnSync(process.execPath, [checker, "--repo-root", root], {
    encoding: "utf8",
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /wallet\/page\.tsx.*mock data source/i);
});
