#!/usr/bin/env node
// Work out who contributed each mirrored material.
//
// The Library used to credit every material to a hard-coded "HENU Kit". These
// are other people's notes, exam papers and lecture slides, so they are
// credited to whoever actually contributed them.
//
// manifest.json records no author, so attribution comes from the repository's
// commit history: the first commit to introduce a path is its contributor. Only
// the public GitHub login is used — never the commit email, which is personal
// data the Library has no reason to carry.
//
// The mirror is a shallow clone with no history, so history is read from the
// GitHub API. Output is a JSON object of { publicPath: login }, written to the
// path given by --out. A failure here must not fail a sync: materials simply
// carry no contributor, and the Portal omits the credit.
//
// Usage:
//   node fetch-henukit-contributors.mjs --out contributors.json \
//     [--repo jry21223/HENU-Final-Review] [--token "$GITHUB_TOKEN"]

import { writeFileSync } from "node:fs";

const DEFAULT_REPO = "jry21223/HENU-Final-Review";

function parseArgs(argv) {
  const options = { repo: DEFAULT_REPO, out: "", token: process.env.GITHUB_TOKEN ?? "" };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--repo" || arg === "--out" || arg === "--token") {
      options[arg.slice(2)] = argv[i + 1] ?? "";
      i += 1;
    }
  }
  return options;
}

async function githubJSON(url, token) {
  const headers = {
    Accept: "application/vnd.github+json",
    "User-Agent": "henukit-materials-contributors",
  };
  if (token) headers.Authorization = `Bearer ${token}`;

  const response = await fetch(url, { headers });
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText} for ${url}`);
  }
  return response.json();
}

/**
 * Maps each path to the login of whoever first added it.
 *
 * Commits are walked oldest-first so a later edit never takes credit from the
 * original contributor. A commit with no linked GitHub account falls back to
 * the name recorded in the commit itself.
 */
export function attributePaths(commitsOldestFirst) {
  const owner = new Map();
  for (const { login, files } of commitsOldestFirst) {
    if (!login) continue;
    for (const path of files) {
      if (!owner.has(path)) owner.set(path, login);
    }
  }
  return Object.fromEntries(owner);
}

async function main() {
  const { repo, out, token } = parseArgs(process.argv.slice(2));
  if (!out) {
    console.error("--out is required");
    process.exit(64);
  }

  const listed = [];
  for (let page = 1; ; page += 1) {
    const batch = await githubJSON(
      `https://api.github.com/repos/${repo}/commits?per_page=100&page=${page}`,
      token
    );
    listed.push(...batch);
    if (batch.length < 100) break;
  }

  // Oldest first: the commit that introduced a path is its contributor.
  listed.reverse();

  const commits = [];
  for (const entry of listed) {
    const detail = await githubJSON(
      `https://api.github.com/repos/${repo}/commits/${entry.sha}`,
      token
    );
    commits.push({
      login: detail.author?.login ?? detail.commit?.author?.name ?? "",
      files: (detail.files ?? []).map((file) => file.filename),
    });
  }

  const attribution = attributePaths(commits);
  writeFileSync(out, JSON.stringify(attribution, null, 2), "utf8");

  const perContributor = {};
  for (const login of Object.values(attribution)) {
    perContributor[login] = (perContributor[login] ?? 0) + 1;
  }
  console.error(
    `attributed ${Object.keys(attribution).length} paths: ${JSON.stringify(perContributor)}`
  );
}

if (process.argv[1] && import.meta.url.endsWith(process.argv[1].split("/").pop())) {
  main().catch((error) => {
    console.error(`contributor attribution failed: ${error.message}`);
    process.exit(1);
  });
}
