import { lstatSync, readlinkSync, symlinkSync, unlinkSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";

export function metadata(path) {
  const info = lstatSync(path);
  return { dev: info.dev, ino: info.ino, uid: info.uid, gid: info.gid, mode: info.mode & 0o7777 };
}

// Explicit test-only equivalent of the separately reviewed operator bootstrap.
// Production tools deliberately cannot perform this migration themselves.
export function forwarderContract(state, releases, inner, outer) {
  try { unlinkSync(outer); } catch (error) { if (error.code !== "ENOENT") throw error; }
  symlinkSync(inner, outer, "dir");
  const value = {
    format: "henukit-active-alias-forwarder-v1",
    release_root: releases,
    inner,
    outer,
    outer_snapshot: { ...metadata(outer), text: readlinkSync(outer) },
    parents: Object.fromEntries([dirname(inner), dirname(outer)].map(path => [path, metadata(path)])),
  };
  const contract = join(state, "active-alias-contract.json");
  try { unlinkSync(contract); } catch (error) { if (error.code !== "ENOENT") throw error; }
  writeFileSync(contract, `${JSON.stringify(value)}\n`, { mode: 0o400 });
  return value;
}
