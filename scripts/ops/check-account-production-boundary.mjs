#!/usr/bin/env node
import {
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { execFileSync } from "node:child_process";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

function die(message) {
  process.stderr.write(`check-account-production-boundary: ${message}\n`);
  process.exit(1);
}

function parseArguments(argv) {
  let repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
  let report = "";
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--repo-root" && argv[index + 1]) {
      repoRoot = resolve(argv[++index]);
    } else if (argument === "--report" && argv[index + 1]) {
      report = resolve(argv[++index]);
    } else {
      die(`unsupported argument ${argument}`);
    }
  }
  return { repoRoot, report };
}

function read(root, path) {
  try {
    return readFileSync(join(root, path), "utf8");
  } catch (error) {
    die(`cannot read ${path}: ${error.message}`);
  }
}

function sourceFiles(root, path) {
  const directory = join(root, path);
  const files = [];
  let entries;
  try {
    entries = readdirSync(directory, { withFileTypes: true });
  } catch (error) {
    die(`cannot inspect ${path}: ${error.message}`);
  }
  for (const entry of entries) {
    const candidate = join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...sourceFiles(root, relative(root, candidate)));
    } else if (entry.isFile() && /\.(?:ts|tsx|js|jsx|mjs|go)$/.test(entry.name)) {
      files.push(candidate);
    }
  }
  return files;
}

function assertAccountSources(root) {
  const entryFiles = sourceFiles(root, "apps/portal/src/app/account/(console)");
  entryFiles.push(
    join(root, "apps/portal/src/components/account/account-console-session.tsx"),
  );
  const files = new Set();
  const pending = [...entryFiles];
  const portalSource = join(root, "apps/portal/src");
  while (pending.length > 0) {
    const file = pending.pop();
    if (files.has(file)) continue;
    files.add(file);
    const source = readFileSync(file, "utf8");
    for (const match of source.matchAll(/(?:from\s*|import\s*)["']([^"']+)["']/g)) {
      const specifier = match[1];
      let base = "";
      if (specifier.startsWith("@/")) base = join(portalSource, specifier.slice(2));
      else if (specifier.startsWith(".")) base = resolve(dirname(file), specifier);
      else continue;
      const candidates = [base, ...[".ts", ".tsx", ".js", ".jsx", ".mjs"].map((extension) => `${base}${extension}`), ...["index.ts", "index.tsx", "index.js"].map((name) => join(base, name))];
      const dependency = candidates.find((candidate) => existsSync(candidate));
      if (!dependency) die(`${relative(root, file)} has an unresolved local import ${specifier}`);
      if (dependency.startsWith(portalSource)) pending.push(dependency);
    }
  }
  const forbidden = [
    { pattern: /@\/lib\/auth\/(?:mock|store)/, label: "mock data source" },
    { pattern: /@\/lib\/[^"']*mock/, label: "mock data source" },
    { pattern: /\b(?:accountStore|mockAllowed|isMockAuthEnabled)\b/, label: "mock data source" },
    { pattern: /\b(?:localStorage|sessionStorage)\b/, label: "browser fixture storage" },
  ];
  for (const file of files) {
    if (/(?:^|\/)(?:mock|mocks|fixtures?)(?:\.|\/)/i.test(relative(root, file))) {
      die(`${relative(root, file)} is reachable mock or fixture code`);
    }
    const relativeFile = relative(root, file);
    const fullSource = readFileSync(file, "utf8");
    const careerSource = relativeFile.startsWith("apps/portal/src/lib/career/");
    const source = relativeFile === "apps/portal/src/lib/api/client.ts" && fullSource.includes("// ---- Account Portfolio ----")
      ? fullSource.slice(fullSource.indexOf("// ---- Account Portfolio ----"), fullSource.indexOf("// ---- Library ----"))
      : careerSource
        ? fullSource.replace(/\bmockAllowed\b/g, "mockAllowedDisabled")
        : fullSource;
    for (const rule of forbidden) {
      if (rule.pattern.test(source)) {
        die(`${relativeFile} reaches a ${rule.label}`);
      }
    }
  }

  const authMock = read(root, "apps/portal/src/lib/auth/mock.ts");
  if (/\b(?:accountStore|AccountData|MEMBERSHIP_PLANS|FREE_MEMBERSHIP|TicketMsg|unreadNotices)\b/.test(authMock)) {
    die("apps/portal/src/lib/auth/mock.ts contains an Account Portfolio fixture");
  }
}

function assertGatewayAccountSources(root) {
  const candidates = [
    ...sourceFiles(root, "services/portal-gateway/internal/accountportfolio"),
    ...sourceFiles(root, "services/console-gateway/internal/accountportfolio"),
  ];
  for (const file of candidates) {
    if (file.endsWith("_test.go")) continue;
    const source = readFileSync(file, "utf8").replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/.*$/gm, "");
    if (/\b(?:mock|fake|fixture)\w*\b/i.test(source) || /fallback\w*\s*(?:success|ok|response)/i.test(source)) {
      die(`${relative(root, file)} contains a user-reachable fake-success source`);
    }
  }
}

function assertBuildBoundary(root) {
  const dockerfile = read(root, "apps/portal/Dockerfile");
  if (!/^ARG NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=1$/m.test(dockerfile)) {
    die("Portal Dockerfile does not require the real Gateway by default");
  }
  if (/NEXT_PUBLIC_PORTAL_ALLOW_MOCK/.test(dockerfile)) {
    die("Portal Dockerfile exposes a production mock build input");
  }

  const compose = read(root, "docker-compose.henukit.yml");
  if (!/PORTAL_API_MODE:\s*\$\{PORTAL_API_MODE:-live\}/.test(compose)) {
    die("HENU Kit Compose does not default Portal API to live mode");
  }

  const workflow = read(root, ".github/workflows/deploy-henukit.yml");
  if (!/henukit-release-images\.sh --github-matrix/.test(workflow)) {
    die("release workflow does not use the canonical image inventory");
  }

  const inventory = join(root, "scripts/ops/henukit-release-images.sh");
  let portalBuildArgs;
  try {
    execFileSync(inventory, ["--check"], { stdio: "pipe" });
    portalBuildArgs = execFileSync(
      inventory,
      ["--field", "portal", "build_args"],
      { encoding: "utf8", stdio: "pipe" },
    );
  } catch (error) {
    die(`cannot validate canonical release image inventory: ${error.message}`);
  }
  if (!/^NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=1$/m.test(portalBuildArgs)) {
    die("release inventory does not build Portal in require-Gateway mode");
  }
  if (/NEXT_PUBLIC_PORTAL_ALLOW_MOCK\s*=\s*1/.test(portalBuildArgs)) {
    die("release inventory enables Portal mock mode");
  }
}

function assertPaymentProviderBoundary(root) {
  const server = read(root, "services/account-portfolio/cmd/server/main.go");
  if (/NewFakePaymentProvider|provider\s*[:=]\s*["']fake["']/i.test(server)) {
    die("Account Portfolio runtime wires a fake payment provider");
  }
  if (!/ACCOUNT_PORTFOLIO_EASYPAY_ENABLED/.test(server) ||
      !/NewEasyPayProvider/.test(server) ||
      !/return\s+nil/.test(server)) {
    die("Account Portfolio runtime is not fail-closed to EasyPay-or-disabled");
  }
}

const { repoRoot, report } = parseArguments(process.argv.slice(2));
try {
  if (!statSync(repoRoot).isDirectory()) die("repository root is not a directory");
} catch (error) {
  die(`repository root is unavailable: ${error.message}`);
}

assertAccountSources(repoRoot);
assertGatewayAccountSources(repoRoot);
assertBuildBoundary(repoRoot);
assertPaymentProviderBoundary(repoRoot);

if (report) {
  const releaseSha = process.env.RELEASE_SHA?.trim() ?? "";
  if (!/^[0-9a-f]{40}$/.test(releaseSha)) {
    die("RELEASE_SHA must be a full lowercase Git SHA when writing a report");
  }
  const contents = [
    `release_sha=${releaseSha}`,
    "status=pass",
    "account_console_mock_sources=absent",
    "account_transitive_mock_sources=absent",
    "account_payment_provider=easypay_or_disabled",
    "portal_require_gateway=1",
    "portal_allow_mock=0",
    "portal_api_default_mode=live",
    "",
  ].join("\n");
  mkdirSync(dirname(report), { recursive: true });
  writeFileSync(report, contents, { mode: 0o644 });
}

process.stdout.write("Account production boundary: PASS (real Gateway required; Account mock sources absent)\n");
