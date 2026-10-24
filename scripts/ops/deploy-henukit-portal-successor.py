#!/usr/bin/env python3
"""Replace only Portal over the active Practice cohort, preserving its rollback."""

import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time

BASE = Path(__file__).with_name("deploy-henukit-food-scoped.py")
# Validate the import before executing it; bootstrap verifies the attestation
# before invoking either file. The inherited provenance gate binds both again.
owner = os.geteuid()
boundary = Path("/")
if "--test-root" in sys.argv:
    boundary = Path(sys.argv[sys.argv.index("--test-root") + 1]).resolve()
    if owner == 0 or boundary == Path(tempfile.gettempdir()).resolve() or not boundary.is_relative_to(Path(tempfile.gettempdir()).resolve()) or not BASE.resolve().is_relative_to(boundary):
        raise SystemExit("portal-successor: invalid test boundary")
for entry in (BASE, *BASE.absolute().parents):
    if entry == boundary:
        break
    info = entry.lstat()
    if entry.is_symlink() or info.st_uid != owner or info.st_mode & 0o022:
        raise SystemExit("portal-successor: untrusted engine path")
spec = importlib.util.spec_from_file_location("henukit_scoped_engine", BASE)
engine = importlib.util.module_from_spec(spec)
spec.loader.exec_module(engine)
require = engine.require

SERVICES = ("portal",)
PREDECESSOR_SCOPE = ("portal", "portal-gateway", "food", "console-gateway", "quizcraft")


class Release(engine.Release):
    services = SERVICES
    helper_file = Path(__file__)
    state_scope = "portal-successor"
    receipt_format = "henukit-portal-successor-v2"
    predecessor_rollbacks = ()

    def database_target(self, containers):
        # Portal has no database; reuse only the engine's configuration checks.
        pass

    def full_baseline(self):
        current = self.path("/opt/henukit/current")
        legacy = self.path("/opt/henukit-current")
        require(all(p.is_symlink() and p.lstat().st_uid == self.uid for p in (current, legacy)), "full aliases must be trusted symlinks")
        runtime = current.resolve()
        require(legacy.resolve() == runtime, "full release aliases disagree")
        for path in (runtime / "RELEASE_SHA", runtime / "docker-compose.henukit.release.yml", self.watch / "last-activated-sha"):
            self.trusted(path)
        sha = (runtime / "RELEASE_SHA").read_text().strip()
        require(re.fullmatch(r"[0-9a-f]{40}", sha) and (self.watch / "last-activated-sha").read_text().strip() == sha, "full baseline marker mismatch")
        return runtime, sha

    def pending(self):
        for folder in (self.watch / "approvals", self.watch / "rollback-contracts/pending"):
            self.trusted_directory(folder)
        pending = []
        for path in (self.watch / "approvals").iterdir():
            if path.name in ("consumed", "revoked"):
                self.trusted_directory(path)
            else:
                pending.append(path)
        require(not pending and not any((self.watch / "rollback-contracts/pending").iterdir()), "pending full release must be reconciled first")

    def predecessor_link(self, data, record, previous):
        require(
            data["predecessor_record"] == str(record)
            and data["predecessor_receipt_sha256"] == engine.digest(record)
            and data["predecessor_runtime"] == previous["runtime"]
            and data["predecessor_sha"] == data["old_version"] == previous["sha"]
            and data["old_deployed_at"] == previous["deployed_at"]
            and data["predecessor_compose_sha256"] == previous["candidate_compose_sha256"],
            "retained predecessor binding changed",
        )
        require(set(data["selected"]) == {"henukit-portal-1"}, "predecessor selection differs")
        selected = data["selected"]["henukit-portal-1"]
        require(selected["tag"] == "henukit-portal:" + previous["sha"] and selected["image"] == previous["images"]["portal"], "retained predecessor Portal image differs")
        if previous["scope"] == list(SERVICES):
            require(data["unselected"] == previous["unselected"], "retained non-Portal containers changed")

    def predecessor_chain(self, record):
        chain, seen = [], set()
        for _ in range(64):
            self.trusted(record, private=True)
            data = json.loads(record.read_text())
            practice = data.get("format") == "henukit-practice-scoped-v1"
            portal = data.get("format") in ("henukit-portal-successor-v1", self.receipt_format)
            scope = PREDECESSOR_SCOPE if practice else SERVICES
            require((practice or portal) and data.get("status") == "active" and data.get("scope") == list(scope), "unsupported predecessor scope/state")
            sha = data["sha"]
            require(re.fullmatch(r"[0-9a-f]{40}", sha) and sha != self.o.sha and record not in seen, "invalid or cyclic predecessor SHA")
            folder = "practice-console" if practice else "portal-successor"
            require(record == self.path("/var/lib/henukit-scoped-releases/" + folder) / sha / "receipt.json", "unexpected predecessor receipt path")
            seen.add(record)
            runtime = Path(data["runtime"])
            helper = "deploy-henukit-practice-scoped.py" if practice else self.helper_file.name
            for name, key in {
                "bin/" + helper: "helper_sha256",
                "bin/deploy-henukit-food-scoped.py": "engine_sha256",
                "docker-compose.henukit.release.yml": "candidate_compose_sha256",
            }.items():
                self.trusted(runtime / name)
                require(engine.digest(runtime / name) == data[key], "predecessor runtime identity changed: " + name)
            self.trusted(runtime / "RELEASE_SHA")
            require((runtime / "RELEASE_SHA").read_text().strip() == sha, "predecessor runtime SHA mismatch")
            require(set(data["images"]) == set(scope), "predecessor image scope mismatch")
            chain.append((record, data))
            if practice:
                break
            backup = record.parent / "configuration.backup"
            self.trusted(backup, private=True)
            require(engine.digest(backup) == data["configuration_backup_sha256"] == data["environment_sha256"], "retained configuration backup changed")
            previous_sha = data["predecessor_sha"]
            require(re.fullmatch(r"[0-9a-f]{40}", previous_sha), "invalid retained predecessor SHA")
            record = Path(data["predecessor_record"])
            require(record in {self.path("/var/lib/henukit-scoped-releases/" + kind) / previous_sha / "receipt.json" for kind in ("practice-console", "portal-successor")}, "unexpected retained predecessor path")
        else:
            raise engine.ReleaseError("Portal predecessor chain exceeds 64 receipts")
        root = chain[-1][1]
        baseline_compose = Path(root["baseline_runtime"]) / "docker-compose.henukit.release.yml"
        self.trusted(baseline_compose)
        baseline_digest = engine.digest(baseline_compose)
        for _, data in chain:
            require(data["baseline_compose_sha256"] == baseline_digest, "retained full baseline Compose changed")
            self.rollback_images({service: {
                "tag": "henukit-" + service + ":" + data["sha"], "image": image,
            } for service, image in data["images"].items()})
            require(data["baseline"] == root["baseline"] and data["baseline_runtime"] == root["baseline_runtime"], "retained full baseline differs")
        for (_, data), (record, previous) in zip(chain, chain[1:]):
            self.predecessor_link(data, record, previous)
        return chain

    def predecessor(self, containers):
        require(self.marker.is_symlink() and self.marker.lstat().st_uid == self.uid, "active scoped marker required")
        chain = self.predecessor_chain(self.marker.resolve() / "receipt.json")
        record, data = chain[0]
        practice = chain[-1][1]
        for service in PREDECESSOR_SCOPE:
            previous = data if service == "portal" else practice
            item = containers["henukit-" + service + "-1"]
            require(item["Config"]["Image"] == "henukit-" + service + ":" + previous["sha"] and item["Image"] == previous["images"][service], "active predecessor image mismatch: " + service)
        if data["scope"] == list(SERVICES):
            require(self.snapshot(containers) == data["unselected"], "active non-Portal containers changed")
        return record, data

    @staticmethod
    def projection(rendered):
        result = {k: rendered.get(k) for k in ("name", "networks", "volumes", "configs", "secrets")}
        result["services"] = {"portal": dict(rendered["services"]["portal"])}
        result["services"]["portal"].pop("image", None)
        return result

    def preflight(self):
        attempt = self.source()
        images = self.provenance()
        baseline, full_sha = self.full_baseline()
        self.pending()
        require(not self.record.parent.exists(), "prior attempt requires explicit rollback or inspection")
        self.trusted(self.envfile, private=True)
        containers = self.containers()
        record, previous = self.predecessor(containers)
        require(previous["baseline"] == full_sha and Path(previous["baseline_runtime"]) == baseline, "predecessor full baseline differs")
        for name, item in containers.items():
            if name in {"henukit-" + s + "-1" for s in engine.DEFERRED}:
                require(not item["State"]["Running"], "deferred dependency must remain stopped")
            else:
                require(item["State"]["Running"] and item["State"].get("Health", {}).get("Status", "healthy") == "healthy", "non-deferred baseline service unhealthy: " + name)
        runtime = Path(previous["runtime"])
        old_sha, deployed = previous["sha"], previous["deployed_at"]
        require(re.fullmatch(r"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ", deployed), "invalid predecessor timestamp")
        before = json.loads(self.compose(runtime, old_sha, old_sha, deployed, "config", "--format", "json"))
        after = json.loads(self.compose(self.runtime, self.o.sha, old_sha, deployed, "config", "--format", "json"))
        require(self.projection(before) == self.projection(after), "Portal or shared resource configuration is incompatible")
        self.running_configuration(containers, runtime, old_sha, old_sha, deployed, before)
        self.rollback_images(self.snapshot(containers, True))
        require(shutil.disk_usage(self.artifacts).free >= 4096 * 1024 * 1024, "less than 4096 MiB free")
        return {
            "format": self.receipt_format, "scope": list(SERVICES), "sha": self.o.sha,
            "run_id": self.o.run_id, "attempt": attempt, "baseline": full_sha,
            "baseline_runtime": str(baseline), "runtime": str(self.runtime),
            "predecessor_record": str(record), "predecessor_receipt_sha256": engine.digest(record),
            "predecessor_runtime": str(runtime), "predecessor_sha": old_sha,
            "predecessor_compose_sha256": engine.digest(runtime / "docker-compose.henukit.release.yml"),
            "old_version": old_sha, "old_deployed_at": deployed,
            "baseline_compose_sha256": engine.digest(baseline / "docker-compose.henukit.release.yml"),
            "candidate_compose_sha256": engine.digest(self.runtime / "docker-compose.henukit.release.yml"),
            "helper_sha256": engine.digest(self.helper_file), "engine_sha256": engine.digest(BASE),
            "selected": self.snapshot(containers, True), "unselected": self.snapshot(containers),
            "image_candidates": images, "images": {}, "environment_sha256": engine.digest(self.envfile),
            "getwork_acceptance": "deferred", "whole_application_release": False,
        }

    def validate(self, data):
        require(data.get("format") == self.receipt_format and data.get("scope") == list(SERVICES) and data.get("sha") == self.o.sha and data.get("run_id") == self.o.run_id, "successor receipt identity mismatch")
        require(data["helper_sha256"] == engine.digest(self.helper_file) and data["engine_sha256"] == engine.digest(BASE), "successor helper identity changed")
        for path, checksum in (
            (self.runtime / "docker-compose.henukit.release.yml", data["candidate_compose_sha256"]),
            (Path(data["baseline_runtime"]) / "docker-compose.henukit.release.yml", data["baseline_compose_sha256"]),
            (Path(data["predecessor_runtime"]) / "docker-compose.henukit.release.yml", data["predecessor_compose_sha256"]),
            (Path(data["predecessor_record"]), data["predecessor_receipt_sha256"]),
        ):
            self.trusted(path)
            require(engine.digest(path) == checksum, "retained release identity changed: " + path.name)
        expected, previous = self.predecessor_chain(Path(data["predecessor_record"]))[0]
        self.predecessor_link(data, expected, previous)
        require(self.marker.is_symlink() and self.marker.lstat().st_uid == self.uid and self.marker.resolve() in (expected.parent, self.record.parent), "foreign scoped marker")
        baseline, sha = self.full_baseline()
        require(str(baseline) == data["baseline_runtime"] and sha == data["baseline"], "full release baseline changed")

    def unchanged(self, data):
        self.validate(data)
        self.trusted(self.envfile, private=True)
        super().unchanged(data)
        self.pending()

    def backup(self, data):
        backup = self.record.parent / "configuration.backup"
        with self.envfile.open("rb") as source, backup.open("xb") as target:
            shutil.copyfileobj(source, target)
            target.flush()
            os.fsync(target.fileno())
        backup.chmod(0o600)
        self.trusted(backup, private=True)
        require(engine.digest(backup) == data["environment_sha256"], "configuration changed during backup")
        self.durable_directory(self.record.parent)
        data["configuration_backup_sha256"] = engine.digest(backup)

    def scope_acceptance(self, data, candidate):
        runtime = self.runtime if candidate else Path(data["predecessor_runtime"])
        sha = data["sha"] if candidate else data["predecessor_sha"]
        deployed = data["deployed_at"] if candidate else data["old_deployed_at"]
        rendered = json.loads(self.compose(runtime, sha, sha, deployed, "config", "--format", "json"))
        self.running_configuration(self.containers(), runtime, sha, sha, deployed, rendered)
        self.http("/api/v1/practice/catalog", (200,))

    def healthy(self, data, candidate):
        sha = data["sha"] if candidate else data["predecessor_sha"]
        expected = data["images"]["portal"] if candidate else data["selected"]["henukit-portal-1"]["image"]
        for _ in range(30):
            portal = self.containers()["henukit-portal-1"]
            if portal["State"]["Running"] and portal["Image"] == expected and portal["Config"]["Image"] == "henukit-portal:" + sha and portal["State"].get("Health", {}).get("Status", "healthy") == "healthy":
                break
            if not self.testing:
                time.sleep(2)
        else:
            raise engine.ReleaseError("Portal did not become ready")
        self.unchanged(data)
        for path in ("/", "/food", "/library", "/practice", "/api/v1/healthz", "/api/v1/library/materials"):
            self.http(path, (200,))
        self.http("/api/v1/account/summary", (200, 401, 403))
        self.http("/quiz/", (404,))
        self.http("/study-api/healthz", (404,))
        self.scope_acceptance(data, candidate)

    def restore(self, data):
        self.unchanged(data)
        self.rollback_images(data["selected"])
        backup = self.record.parent / "configuration.backup"
        self.trusted(backup, private=True)
        require(engine.digest(backup) == data["environment_sha256"] == data["configuration_backup_sha256"], "current configuration backup changed")
        self.compose(Path(data["predecessor_runtime"]), data["predecessor_sha"], data["old_version"], data["old_deployed_at"], "up", "-d", "--no-deps", "portal")
        self.healthy(data, False)
        temporary = self.watch / (".portal-successor-rollback-" + secrets.token_hex(8))
        temporary.symlink_to(Path(data["predecessor_record"]).parent)
        os.replace(temporary, self.marker)
        self.durable_directory(self.watch)
        self.write_receipt(data, "rolled_back")

    def rollback(self):
        self.trusted(self.record, private=True)
        data = json.loads(self.record.read_text())
        require(data.get("status") in ("preparing", "prepared", "applying", "active", "rollback_failed", "rolled_back"), "unsupported successor state")
        with self.lock_watcher():
            self.unchanged(data)
            if data["status"] in ("preparing", "prepared"):
                require(self.marker.resolve() == Path(data["predecessor_record"]).parent and self.snapshot(self.containers(), True) == data["selected"], "pre-activation intent no longer matches original Portal")
                self.healthy(data, False)
                self.write_receipt(data, "rolled_back")
                return "rolled_back"
            if data["status"] == "rolled_back":
                require(self.marker.resolve() == Path(data["predecessor_record"]).parent, "rolled-back marker differs")
                self.healthy(data, False)
                return "already_rolled_back"
            try:
                self.restore(data)
            except Exception:
                self.write_receipt(data, "rollback_failed")
                raise
        return "rolled_back"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("sha", "run-id", "proof-dir"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--test-root")
    parser.add_argument("--acknowledge-getwork-deferred", action="store_true", required=True)
    modes = parser.add_mutually_exclusive_group(required=True)
    for name in ("preflight", "execute", "rollback"):
        modes.add_argument("--" + name, action="store_true")
    options = parser.parse_args()
    os.umask(0o077)
    require(re.fullmatch(r"[0-9a-f]{40}", options.sha) and options.run_id.isdigit(), "invalid release identity")
    release = Release(options)
    if options.preflight:
        release.preflight()
        status = "preflight_passed"
    else:
        status = release.execute() if options.execute else release.rollback()
    print(json.dumps({"status": status, "scope": list(SERVICES), "getwork_acceptance": "deferred"}))


if __name__ == "__main__":
    try:
        main()
    except (engine.ReleaseError, KeyError, OSError, ValueError, subprocess.TimeoutExpired, tarfile.TarError) as error:
        print("portal-successor: " + str(error), file=sys.stderr)
        sys.exit(1)
