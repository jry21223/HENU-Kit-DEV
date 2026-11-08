#!/usr/bin/env python3
"""Adopt the audited mixed stack as one exact-SHA, getWork-disabled release."""

import argparse
import base64
import contextlib
import copy
from datetime import datetime
import fcntl
import importlib.util
import hashlib
import hmac
import ipaddress
import json
import os
from pathlib import Path
import pwd
import re
import secrets
import shutil
import shlex
import stat
import subprocess
import sys
import tempfile
import time
import tarfile
import io
from urllib.parse import parse_qsl, unquote, urlsplit


def options():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--sha", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--proof-dir", required=True)
    parser.add_argument("--test-root")
    parser.add_argument("--recover-transition")
    parser.add_argument("--recovery-proof-dir")
    parser.add_argument("--materials-baseline-sha")
    parser.add_argument("--materials-baseline-runtime")
    parser.add_argument("--materials-baseline-proof-dir")
    parser.add_argument(
        "--acknowledge-getwork-disabled", action="store_true", required=True
    )
    mode = parser.add_mutually_exclusive_group(required=True)
    for name in ("preflight", "execute", "rollback"):
        mode.add_argument("--" + name, action="store_true")
    result = parser.parse_args()
    if bool(result.recover_transition) != bool(result.recovery_proof_dir):
        parser.error(
            "recovery requires the pinned transition and its proof directory together"
        )
    if result.recover_transition:
        if (
            not result.rollback
            or result.recover_transition != FAILED_SHA
            or result.sha == FAILED_SHA
        ):
            parser.error("only a successor may roll back the pinned failed transition")
        proof = Path(result.recovery_proof_dir)
        if not proof.is_absolute():
            parser.error("recovery proof directory must be absolute")
        result.recovery_proof_dir = str(proof)
    if not re.fullmatch(r"[0-9a-f]{40}", result.sha) or not re.fullmatch(
        r"[1-9][0-9]*", result.run_id
    ):
        parser.error("a full lowercase SHA and numeric Actions run ID are required")
    selection = (
        result.materials_baseline_sha,
        result.materials_baseline_runtime,
        result.materials_baseline_proof_dir,
    )
    if any(selection) and not all(selection):
        parser.error(
            "independent materials baseline requires SHA, runtime and proof directory together"
        )
    if result.materials_baseline_sha and not re.fullmatch(
        r"[0-9a-f]{40}", result.materials_baseline_sha
    ):
        parser.error("materials baseline SHA must be a full lowercase Git SHA")
    if result.materials_baseline_sha:
        for name in ("materials_baseline_runtime", "materials_baseline_proof_dir"):
            value = Path(getattr(result, name))
            if not value.is_absolute():
                parser.error(
                    "materials baseline runtime and proof directory must be absolute"
                )
            setattr(result, name, str(value))
    return result


FAILED_SHA = "c97d7e93db5b9f12b137e610b7e22bf6b911392e"
FAILED_HELPER = "007f55b1b852d8276ad18b8f9fa4f0576dcf368c806a0dc7825eb13efee92cbd"
FAILED_RUN = "41369983145"
SELECTED = options() if __name__ == "__main__" else None
sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parent
BOUNDARY = Path("/")
if "--test-root" in sys.argv:
    BOUNDARY = Path(sys.argv[sys.argv.index("--test-root") + 1]).resolve()


def dependency(name):
    path = ROOT / name
    for entry in (path, *path.parents):
        if entry == BOUNDARY:
            break
        info = entry.lstat()
        if entry.is_symlink() or info.st_uid != os.geteuid() or info.st_mode & 0o022:
            raise SystemExit("untrusted stack-transition dependency")
    spec = importlib.util.spec_from_file_location(name.replace(".", "_"), path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


engine = dependency("deploy-henukit-food-scoped.py")
portal = dependency("deploy-henukit-portal-successor.py")
practice = dependency("deploy-henukit-practice-scoped.py")
membership = dependency("deploy-henukit-account-membership.py")
require = engine.require
IMAGES = (
    "platform-core",
    "platform-smtp-provider",
    "platform-mail-worker",
    "portal-api",
    "account-portfolio",
    "notice",
    "notice-worker",
    "food",
    "food-mcp",
    "career-mcp",
    "career-opportunities",
    "library",
    "portal-gateway",
    "quizcraft",
    "portal",
    "portal-summary",
    "console-gateway",
    "console",
)
SCOPE = (*IMAGES, "nginx")
EXCLUDED = ("getwork-mcp", "getwork-mcp-relay")
OWNERS = {
    "platform-core": ("platform-core", "PLATFORM_CORE_DATABASE_URL", "platform"),
    "account-portfolio": (
        "account-portfolio",
        "ACCOUNT_PORTFOLIO_DATABASE_URL",
        "account_portfolio",
    ),
    "notice": ("notice", "NOTICE_DATABASE_URL", "notice"),
    "food": ("food", "FOOD_DATABASE_URL", "food"),
    "library": ("library", "LIBRARY_DATABASE_URL", "library"),
    "career": ("career-opportunities", "CAREER_DATABASE_URL", "career"),
    "portal": ("portal-api", "PORTAL_DATABASE_URL", "portal"),
}
FORMAT = "henukit-stack-transition-v1"


class RecoveredPortalGuard(portal.Release):
    """Compare history with history, and real recreated IDs with their receipt."""

    def __init__(self, options, terminal):
        super().__init__(options)
        self.original_checkpoint = {**terminal["unselected"], **terminal["selected"]}
        self.live_checkpoint = {
            **terminal["unselected"],
            **terminal["rollback_checkpoint"],
        }

    def live_checkpoint_matches(self, actual, expected):
        names = set(expected)
        return (
            set(actual) == names
            and names <= self.original_checkpoint.keys()
            and expected == {name: self.original_checkpoint[name] for name in names}
            and actual == {name: self.live_checkpoint[name] for name in names}
        )


class Release(engine.Release):
    services = SCOPE
    helper_file = Path(__file__)
    state_scope = "stack-transition"
    receipt_format = FORMAT

    @staticmethod
    def alias_metadata(path):
        info = path.lstat()
        return dict(dev=info.st_dev, ino=info.st_ino, uid=info.st_uid,
                    gid=info.st_gid, mode=stat.S_IMODE(info.st_mode))

    def alias_snapshot(self, path):
        self.trusted_directory(path.parent)
        require(path.is_symlink() and path.lstat().st_uid == self.uid, "full alias is not an owned symlink")
        return {**self.alias_metadata(path), "text": os.readlink(path), "mtime_ns": path.lstat().st_mtime_ns}

    def alias_contract(self):
        path = self.watch / "active-alias-contract.json"
        inner, outer = self.path("/opt/henukit/current"), self.path("/opt/henukit-current")
        self.trusted(path, private=True)
        require(stat.S_IMODE(path.lstat().st_mode) == 0o400, "full alias contract must be immutable")
        value = json.loads(path.read_bytes())
        require(set(value) == {"format", "release_root", "inner", "outer", "outer_snapshot", "parents"}
                and value["format"] == "henukit-active-alias-forwarder-v1"
                and (value["release_root"], value["inner"], value["outer"]) ==
                    tuple(map(str, (self.path("/opt/henukit-releases"), inner, outer))),
                "full alias topology requires an explicit versioned bootstrap")
        require(value["outer_snapshot"] == {**self.alias_metadata(outer), "text": os.readlink(outer)}
                and self.alias_snapshot(outer)["text"] == str(inner), "fixed outer full alias changed")
        for parent in (inner.parent, outer.parent):
            self.trusted_directory(parent)
        require(value["parents"] == {str(p): self.alias_metadata(p) for p in (inner.parent, outer.parent)},
                "full alias parent identity changed")
        return {"identity": self.alias_metadata(path), "sha256": engine.digest(path), "source": self.alias_source_proof()}

    def alias_source_proof(self):
        config = self.path("/etc/henukit-deploy/deploy.env")
        source = self.path("/run/lock/henukit-deploy.lock")
        self.trusted(config)
        config_info = config.lstat()
        config_group = pwd.getpwnam("henukit-deploy").pw_gid if self.prefix == Path("/") else os.getegid()
        require(stat.S_IMODE(config_info.st_mode) in (0o400, 0o600) or
                (stat.S_IMODE(config_info.st_mode) == 0o640 and config_info.st_gid == config_group),
                "source deploy config differs from its protected authority profile")
        configured = "/run/lock/henukit-deploy.lock"
        for line in config.read_text().splitlines():
            match = re.match(r"\s*(?:export\s+)?HENUKIT_DEPLOY_LOCK_FILE\s*=(.*)$", line)
            if match:
                values = shlex.split(match.group(1), comments=True)
                require(len(values) == 1, "source lock config is not literal")
                configured = values[0]
        require(configured == str(source), "source protocol lock differs from its original config")
        info, parent = source.lstat(), source.parent.lstat()
        if self.prefix == Path("/"):
            account = pwd.getpwnam("henukit-deploy")
            owner, group = account.pw_uid, account.pw_gid
            require(stat.S_ISDIR(parent.st_mode) and parent.st_uid == 0 and stat.S_IMODE(parent.st_mode) == 0o1777,
                    "source lock parent differs")
        else:
            self.trusted_directory(source.parent)
            owner, group = self.uid, os.getegid()
        require(stat.S_ISREG(info.st_mode) and info.st_uid == owner and info.st_gid == group
                and stat.S_IMODE(info.st_mode) == 0o640 and info.st_size == 0, "original source lock is unavailable")
        return {"file": self.alias_metadata(source), "parent": self.alias_metadata(source.parent),
                "config": self.alias_metadata(config), "config_sha256": engine.digest(config)}

    @contextlib.contextmanager
    def lock_watcher(self):
        # Keep the original outer direct→watcher protocol, then coordinate with
        # the existing directory adopter and retired source writer. Never create
        # or change their lock files, users, services or queue.
        with super().lock_watcher():
            frozen = self.alias_source_proof()
            held = []
            try:
                for path, directory in ((self.watch, True), (self.path("/run/lock/henukit-deploy.lock"), False)):
                    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | (os.O_DIRECTORY if directory else 0))
                    held.append((fd, path))
                    fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    current = path.lstat()
                    require((os.fstat(fd).st_dev, os.fstat(fd).st_ino) == (current.st_dev, current.st_ino), "full alias lease identity changed")
                require(self.alias_source_proof() == frozen, "source coordination changed before lease")
                self._alias_leases = [(fd, path, self.alias_metadata(path)) for fd, path in held]
                yield
            finally:
                self._alias_leases = []
                for fd, _ in reversed(held): os.close(fd)

    def alias_journal(self, name):
        path = self.record.parent / name
        if not path.exists():
            require(not path.is_symlink(), "full alias journal is a dangling symlink")
            return None
        self.trusted(path, private=True)
        require(stat.S_IMODE(path.lstat().st_mode) == 0o400, "full alias journal lost immutable mode")
        return json.loads(path.read_bytes())

    def verify_alias_authority(self, data):
        for fd, path, frozen in getattr(self, "_alias_leases", []):
            current = os.fstat(fd)
            held = dict(dev=current.st_dev, ino=current.st_ino, uid=current.st_uid, gid=current.st_gid, mode=stat.S_IMODE(current.st_mode))
            require(held == frozen == self.alias_metadata(path), "full alias lease authority changed")
        if self.terminal_alias_authority is not None:
            authority = self.terminal_alias_authority
            self.pinned_receipt(data)
            require(data["status"] == "rolled_back" and self.recovery_authority,
                    "read-only historical verification requires a completed pinned recovery")
            self.recovery_unchanged(data)
            require(engine.digest(self.record) == authority["receipt_sha256"]
                    and engine.digest(Path(authority["audit"])) == authority["audit_sha256"]
                    and self.alias_contract() == authority["alias_contract"],
                    "historical evidence or bootstrap identity changed")
            inner, outer = self.path("/opt/henukit/current"), self.path("/opt/henukit-current")
            current = self.alias_snapshot(inner)
            require(current["text"] == data["baseline_runtime"] and str(outer.resolve()) == data["baseline_runtime"],
                    "versioned aliases do not own the recovered historical baseline")
            return current
        if self.legacy_alias_recovery(data):
            return None
        require(self.alias_contract() == data.get("alias_contract"), "full alias contract changed or legacy receipt needs migration")
        current = self.alias_snapshot(self.path("/opt/henukit/current"))
        known = [data["full_alias_before"]]
        for name in ("full-alias-commit.json", "full-alias-rollback.json"):
            record = self.alias_journal(name)
            if record:
                require(record["binding"] == self.alias_binding(data), "full alias journal belongs to another transition")
                known.append(record["link"])
        require(current in known, "foreign full alias inode prevents transition writes")
        return current

    def legacy_alias_recovery(self, data):
        require(self.terminal_alias_authority is None, "read-only history cannot authorize legacy writes")
        if "alias_contract" in data or "full_alias_before" in data:
            return False
        require(self.recovery, "legacy receipt requires the original explicit pinned recovery")
        for path in (self.watch / "active-alias-contract.json", self.watch / "active-alias-bootstrap", self.watch / "active-aliases"):
            # lstat also rejects dangling links and propagates access/I/O errors;
            # deleting the contract cannot downgrade a started migration.
            try: path.lstat()
            except FileNotFoundError: pass
            else: raise engine.ReleaseError("legacy recovery is forbidden after alias migration authority exists")
        self.pinned_receipt(data)
        require(self.recovery_authority, "legacy recovery has no verified immutable authority")
        self.recovery_unchanged(data)
        for path in (self.path("/opt/henukit/current"), self.path("/opt/henukit-current")):
            snapshot = self.alias_snapshot(path)
            require(snapshot["text"] in (data["baseline_runtime"], data["runtime"]),
                    "legacy recovery requires the original protected direct aliases")
        return True

    @staticmethod
    def alias_binding(data):
        return hashlib.sha256(json.dumps({key: data[key] for key in
            ("sha", "run_id", "helper_sha256", "alias_contract", "full_alias_before", "baseline_runtime", "runtime")}, sort_keys=True).encode()).hexdigest()

    def publish_full_alias(self, data, target, restoring=False):
        self.require_mutable_recovery()
        self.verify_alias_authority(data)
        if self.legacy_alias_recovery(data):
            require(restoring and str(target) == data["baseline_runtime"], "legacy aliases may only restore the original baseline")
            for path in (self.path("/opt/henukit/current"), self.path("/opt/henukit-current")):
                self.verify_alias_authority(data)
                self.alias(path, target)
            return
        inner = self.path("/opt/henukit/current")
        name = "full-alias-rollback.json" if restoring else "full-alias-commit.json"
        record = self.alias_journal(name)
        if record is None:
            temporary = inner.parent / (".transition-full-alias-" + secrets.token_hex(12))
            temporary.symlink_to(target)
            original = data["full_alias_before"]
            if temporary.lstat().st_gid != original["gid"]:
                os.lchown(temporary, self.uid, original["gid"])
            self.durable_directory(inner.parent)
            record = {"binding": self.alias_binding(data), "path": str(temporary), "link": self.alias_snapshot(temporary)}
            incoming = self.record.parent / (".full-alias-journal-" + secrets.token_hex(12))
            descriptor = os.open(incoming, os.O_CREAT | os.O_EXCL | os.O_WRONLY | os.O_NOFOLLOW, 0o400)
            with os.fdopen(descriptor, "wb") as stream:
                stream.write(json.dumps(record, sort_keys=True).encode()); stream.flush(); os.fsync(stream.fileno())
            os.link(incoming, self.record.parent / name, follow_symlinks=False)
            self.durable_directory(self.record.parent)
            incoming.unlink(); self.durable_directory(self.record.parent)
        temporary = Path(record["path"])
        require(record["binding"] == self.alias_binding(data) and temporary.parent == inner.parent
                and temporary.name.startswith(".transition-full-alias-") and record["link"]["text"] == str(target),
                "prepared full alias identity differs")
        before = self.verify_alias_authority(data)
        if before != record["link"]:
            require(temporary.is_symlink() and self.alias_snapshot(temporary) == record["link"], "prepared full alias inode is unavailable")
            require(self.verify_alias_authority(data) == before and self.alias_snapshot(inner) == before,
                    "full alias changed before replacement")
            os.replace(temporary, inner)
        self.durable_directory(inner.parent)
        require(self.alias_snapshot(inner) == record["link"], "full alias replacement ownership is unproved")

    def __init__(self, options, *, terminal_alias_authority=None):
        super().__init__(options)
        self.terminal_alias_authority = terminal_alias_authority
        self.guard = portal.Release(options)
        self.recovery = bool(options.recover_transition)
        self.recovery_authority = None
        if self.recovery:
            self.record = self.state / FAILED_SHA / "receipt.json"
            token = self.path("/etc/henukit/github-actions-read.token")
            self.trusted(token, private=True)
            self.env["GH_TOKEN"] = token.read_text().strip()
            require(self.env["GH_TOKEN"], "existing Actions read token is empty")

    def require_mutable_recovery(self):
        require(self.terminal_alias_authority is None, "read-only historical verification cannot mutate the transition")

    def active_chain(self):
        require(self.marker.is_symlink(), "an active Portal v3 checkpoint is required")
        record = self.marker.resolve() / "receipt.json"
        self.trusted(record, private=True)
        value = json.loads(record.read_text())
        require(
            value.get("format") == "henukit-portal-successor-v3"
            and value.get("status") == "active",
            "an active Portal v3 checkpoint is required",
        )
        return record, value

    def adopt_recovered_checkpoint(self):
        record = self.state / FAILED_SHA / "receipt.json"
        if not record.exists():
            return None
        self.trusted(record, private=True)
        data = json.loads(record.read_text())
        require(
            data.get("status") == "rolled_back",
            "pinned prior transition must finish successor recovery first",
        )
        reference = data.get("recovery_authority", {}).get(self.o.sha)
        require(reference, "current successor has no pinned rollback recovery audit")
        require(
            re.fullmatch(
                "recovery-" + self.o.sha + r"-[1-9][0-9]*\.json",
                reference.get("file", ""),
            ),
            "invalid recovery audit path",
        )
        audit = record.parent / reference["file"]
        self.trusted(audit, private=True)
        require(engine.digest(audit) == reference["sha256"], "recovery audit changed")
        saved = json.loads(audit.read_text())["authority"]
        selected = argparse.Namespace(**vars(self.o))
        selected.rollback = True
        selected.recover_transition = FAILED_SHA
        selected.recovery_proof_dir = saved["target"]["proof_dir"]
        terminal_alias_authority = {
            "receipt_sha256": engine.digest(record), "audit": str(audit),
            "audit_sha256": engine.digest(audit), "alias_contract": self.alias_contract(),
        }
        recovery = Release(selected, terminal_alias_authority=terminal_alias_authority)
        authority = recovery.prepare_recovery(data, Path(selected.recovery_proof_dir))
        require(
            authority == saved,
            "terminal recovery authority differs from verified successor",
        )
        recovery.final_state(data, False)
        recovery.health(data, False)
        recovery.materials_matches(data, False)
        self.guard = RecoveredPortalGuard(self.o, data)
        return {
            "record": str(record),
            "receipt_sha256": engine.digest(record),
            "audit": str(audit),
            "audit_sha256": engine.digest(audit),
            "target": authority["target"],
            "successor": authority["successor"],
            "alias_transition": {**terminal_alias_authority,
                "inner_snapshot": self.alias_snapshot(self.path("/opt/henukit/current")),
                "baseline_runtime": data["baseline_runtime"]},
        }

    def preflight(self):
        alias_contract = self.alias_contract()
        recovered = self.adopt_recovered_checkpoint()
        record, current = self.active_chain()
        require(
            not self.record.parent.exists(),
            "existing transition requires recovery or inspection",
        )
        attempt = self.source()
        images = self.provenance()
        baseline, baseline_sha = self.guard.full_baseline()
        self.guard.pending()
        containers = self.containers()
        self.public_acceptance(containers)
        self.guard.predecessor(containers)
        chain = self.guard.predecessor_chain(record)
        anchor = current.get("membership_checkpoint")
        require(anchor, "Portal v3 membership checkpoint is missing")
        member = self.guard.membership_checkpoint(anchor)
        prior = chain[-1][1]
        self.trusted(self.envfile, private=True)
        environment_sha256 = engine.digest(self.envfile)
        application = self.application(prior)
        before, sources = self.mixed_configuration(
            baseline, baseline_sha, current, prior, member, application
        )
        candidate_deployed_at = self.timestamp()
        after = self.render(
            self.runtime,
            self.o.sha,
            self.o.sha,
            candidate_deployed_at,
            application,
            candidate=True,
        )
        expected_services = set(SCOPE) | {"postgres", "redis"}
        require(
            set(after["services"]) == expected_services,
            "candidate runtime has an unknown or missing service scope",
        )
        require(
            set(before["services"]) - set(EXCLUDED) == expected_services,
            "mixed runtime has an unknown or missing service scope",
        )
        known = {
            "henukit-" + name + "-1"
            for name in (*SCOPE, *EXCLUDED, "postgres", "redis")
        }
        require(not set(containers) - known, "unreviewed current containers exist")
        for name in known - {"henukit-" + s + "-1" for s in EXCLUDED}:
            require(name in containers, "current container missing: " + name)
        for name, item in containers.items():
            service = name.removeprefix("henukit-").removesuffix("-1")
            if service in (*EXCLUDED, "career-opportunities", "career-mcp"):
                require(
                    not item["State"]["Running"],
                    "original Career/getWork cohort must be stopped at the first transition",
                )
            else:
                require(
                    item["State"]["Running"]
                    and item["State"].get("Health", {}).get("Status", "healthy")
                    == "healthy",
                    "current service unhealthy: " + service,
                )
        self.configuration_matches(containers, before, sources=sources)
        self.rollback_images(self.snapshot(containers, True))
        migrations = self.migrations(containers, before, baseline, prior, member)
        self.preserve_configuration(before, after, containers)
        materials = self.materials_preflight(baseline)
        self.database_targets(containers)
        self.practice_helper = practice.Release(self.o)
        self.practice_helper.database_target(containers)
        technical_gates = self.technical_gates(baseline)
        require(
            shutil.disk_usage(self.artifacts).free >= 4096 * 1024 * 1024,
            "less than 4096 MiB free",
        )
        retained = {str(p): engine.digest(p) for p, _ in chain}
        retained[anchor["record"]] = anchor["sha256"]
        retained_files = {}
        if recovered:
            retained[recovered["record"]] = recovered["receipt_sha256"]
            retained_files[recovered["audit"]] = recovered["audit_sha256"]
        for name, checksum in member["configuration_hashes"].items():
            path = Path(anchor["record"]).parent / name
            self.trusted(path, private=True)
            require(
                engine.digest(path) == checksum,
                "membership private configuration changed",
            )
            retained_files[str(path)] = checksum
        for old_record, _ in (*chain, (Path(anchor["record"]), member)):
            path = old_record.parent / "configuration.backup"
            if path.exists():
                self.trusted(path, private=True)
                retained_files[str(path)] = engine.digest(path)
        retained_runtimes = {
            str(p): self.runtime_inventory(p)
            for p in {
                baseline,
                Path(prior["runtime"]),
                Path(current["runtime"]),
                Path(member["runtime"]),
                *(Path(value["runtime"]) for value in migrations.values()),
            }
        }
        require(
            engine.digest(self.envfile) == environment_sha256,
            "production environment changed during preflight",
        )
        return {
            "format": FORMAT,
            "scope": list(SCOPE),
            "sha": self.o.sha,
            "run_id": self.o.run_id,
            "attempt": attempt,
            "runtime": str(self.runtime),
            "helper_sha256": engine.digest(self.helper_file),
            "alias_contract": alias_contract,
            "full_alias_before": self.alias_snapshot(self.path("/opt/henukit/current")),
            "baseline": baseline_sha,
            "baseline_runtime": str(baseline),
            "deployed_at": candidate_deployed_at,
            "environment_sha256": environment_sha256,
            "marker_before": str(record.parent),
            "retained_receipts": retained,
            "retained_runtimes": retained_runtimes,
            "retained_files": retained_files,
            "selected": self.snapshot(containers, True),
            "unselected": self.snapshot(containers),
            "before": before,
            "after": after,
            "sources": sources,
            "before_environment": {
                s: self.container_environment(containers["henukit-" + s + "-1"])
                for s in SCOPE
            },
            "candidate_environment": {
                s: {
                    **self.image_environment.get(
                        s,
                        {
                            key: value
                            for key, value in self.container_environment(
                                containers["henukit-" + s + "-1"]
                            ).items()
                            if key not in before["services"][s].get("environment", {})
                        },
                    ),
                    **{
                        k: str(v)
                        for k, v in after["services"][s].get("environment", {}).items()
                    },
                }
                for s in SCOPE
            },
            "technical_gates": technical_gates,
            "image_candidates": images,
            "images": {},
            "materials": materials,
            "migrations": migrations,
            "application_secret_sha256": prior["application_secret_sha256"],
            "quizcraft_identity": self.practice_helper.database_identity,
            "runtime_hashes": {
                str(p.relative_to(self.runtime)): engine.digest(p)
                for p in self.runtime.rglob("*")
                if p.is_file()
            },
            "application_file": str(
                Path(chain[-1][0]).parent / "quizcraft-console-application.json"
            ),
            "getwork_acceptance": "explicitly_disabled",
            "whole_application_release": True,
            **({"recovered_transition": recovered} if recovered else {}),
        }

    @staticmethod
    def timestamp():
        return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())

    def full_deployment_time(self):
        values = re.findall(
            r"^[ \t]*PORTAL_DEPLOYED_AT[ \t]*=(.*)$", self.envfile.read_text(), re.M
        )
        require(len(values) == 1, "full release timestamp must be uniquely configured")
        value = values[0].strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "'\"":
            value = value[1:-1]
        require(
            re.fullmatch(
                r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z", value
            ),
            "full release timestamp must be a valid UTC timestamp",
        )
        try:
            datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ")
        except ValueError as error:
            raise engine.ReleaseError(
                "full release timestamp must be a valid UTC timestamp"
            ) from error
        return value

    def provenance(self):
        self.services = IMAGES
        try:
            images = super().provenance()
        finally:
            self.services = SCOPE
        # Existing v1 attestation binds getWork+runtime. Other images retain
        # the authenticated exact-run staging/checksum/tag/config/OCI boundary.
        self.image_environment = {}
        for service in IMAGES:
            path = self.archive(
                "henukit-" + service + "-" + self.o.sha + ".docker.tar.gz"
            )
            with self.safe_tar(path) as archive:
                descriptor = json.load(archive.extractfile("manifest.json"))[0]
                config = json.load(archive.extractfile(descriptor["Config"]))
                self.image_environment[service] = dict(
                    v.split("=", 1)
                    for v in config.get("config", {}).get("Env", []) or []
                )
        # Every byte executed/imported or installed by this entry must be in the
        # same attested runtime, including private-configuration parsers.
        runtime = self.archive("henukit-runtime-" + self.o.sha + ".tar.gz")
        with self.safe_tar(runtime) as bundle:
            packaged = {
                m.name.removeprefix("./") for m in bundle.getmembers() if m.isfile()
            }
            actual = {
                str(p.relative_to(self.runtime))
                for p in self.runtime.rglob("*")
                if p.is_file()
            }
            require(
                packaged == actual,
                "executed runtime inventory differs from attestation",
            )
            for name in sorted(packaged):
                path = self.runtime / name
                self.trusted(path)
                raw = bundle.extractfile("./" + name).read()
                require(
                    hashlib.sha256(raw).hexdigest() == engine.digest(path),
                    "runtime differs from attestation: " + name,
                )
        return images

    def runtime_inventory(self, runtime):
        self.trusted_directory(runtime)
        result = {}
        for path in runtime.rglob("*"):
            if path.is_dir() and not path.is_symlink():
                self.trusted_directory(path)
            else:
                self.trusted(path)
                result[str(path.relative_to(runtime))] = engine.digest(path)
        require(result, "retained runtime is empty")
        return result

    def application(self, receipt):
        root = Path(receipt["runtime"])
        records = self.guard.predecessor_chain(
            Path(self.marker.resolve()) / "receipt.json"
        )
        path = records[-1][0].parent / "quizcraft-console-application.json"
        self.trusted(path, private=True)
        require(
            engine.digest(path) == receipt.get("application_secret_sha256"),
            "Practice private application source changed",
        )
        value = json.loads(path.read_text())
        require(
            value.get("client_id") == practice.CLIENT
            and value.get("key_id") == practice.KEY_ID
            and re.fullmatch(r"[A-Za-z0-9_-]{64}", value.get("secret", "")),
            "invalid retained Practice application",
        )
        self.trusted(root / "bin/deploy-henukit-practice-scoped.py")
        self._application = practice.Release.application_environment(
            self, value["secret"]
        )
        return self._application

    def render(
        self, runtime, sha, version, deployed, application=None, candidate=False
    ):
        original = self.env
        self.env = {**original, **(application or {})}
        if candidate:
            self.env["CAREER_GETWORK_ENABLED"] = "0"
        try:
            return json.loads(
                self.compose(
                    runtime, sha, version, deployed, "config", "--format", "json"
                )
            )
        finally:
            self.env = original

    @staticmethod
    def merge(target, addition, services):
        for key in ("name", "networks", "volumes", "configs", "secrets"):
            if key not in addition:
                continue
            if key == "name":
                require(
                    target.get(key) == addition[key],
                    "mixed Compose project identity differs",
                )
                continue
            values = target.setdefault(key, {})
            for name, value in addition[key].items():
                require(
                    name not in values or values[name] == value,
                    "mixed Compose resource conflict: " + name,
                )
                values[name] = copy.deepcopy(value)
        for name in services:
            require(name in addition["services"], "missing rollback service: " + name)
            target["services"][name] = copy.deepcopy(addition["services"][name])

    def mixed_configuration(self, baseline, sha, current, prior, member, application):
        deployed = self.full_deployment_time()
        before = self.render(baseline, sha, sha, deployed)
        sources = {
            s: {
                "runtime": str(baseline),
                "sha": sha,
                "deployed": deployed,
                "application": False,
            }
            for s in before["services"]
        }
        for receipt, selected, app in (
            (prior, practice.SERVICES, application),
            (current, ("portal",), None),
        ):
            rendered = self.render(
                Path(receipt["runtime"]),
                receipt["sha"],
                receipt["sha"],
                receipt["deployed_at"],
                app,
            )
            self.merge(before, rendered, selected)
            for name in selected:
                sources[name] = {
                    "runtime": receipt["runtime"],
                    "sha": receipt["sha"],
                    "deployed": receipt["deployed_at"],
                    "application": bool(app),
                }
        member_record = Path(current["membership_checkpoint"]["record"])
        private = member_record.parent / "after.compose.json"
        # Render the retained JSON again: $$ is Compose literal escaping, not
        # the actual application secret. Copying this raw JSON would double it.
        rendered = json.loads(
            self.run("docker", "compose", "-f", private, "config", "--format", "json")
        )
        self.merge(before, rendered, membership.SERVICES)
        for name in membership.SERVICES:
            sources[name] = {"private": str(private)}
        # Career was intentionally stopped after a later scoped image. Its real
        # retained source, rather than the full alias, owns rollback/schema facts.
        for name in ("career-opportunities", "career-mcp"):
            item = self.containers()["henukit-" + name + "-1"]
            tag = item["Config"]["Image"]
            match = re.fullmatch("henukit-" + name + r":([0-9a-f]{40})", tag)
            require(match, "Career image has no exact source SHA")
            if match[1] != sha:
                runtime = self.path("/opt/henukit-releases") / match[1]
                self.trusted(runtime / "RELEASE_SHA")
                require(
                    (runtime / "RELEASE_SHA").read_text().strip() == match[1],
                    "Career retained runtime identity differs",
                )
                rendered = self.render(runtime, match[1], match[1], deployed)
                self.merge(before, rendered, (name,))
                sources[name] = {
                    "runtime": str(runtime),
                    "sha": match[1],
                    "deployed": deployed,
                    "application": False,
                }
        return before, sources

    def configuration_matches(
        self, containers, config, sources=None, compose_file=None
    ):
        for service in SCOPE:
            item = containers["henukit-" + service + "-1"]
            spec = config["services"][service]
            actual = self.container_environment(item)
            require(
                item["Config"]["Image"] == spec["image"],
                "current image differs from rollback configuration: " + service,
            )
            require(
                all(
                    actual.get(k) == str(v)
                    for k, v in spec.get("environment", {}).items()
                ),
                "current private environment differs: " + service,
            )
            if compose_file:
                output = self.run(
                    "docker", "compose", "-f", compose_file, "config", "--hash", service
                )
            else:
                source = sources[service]
                if "private" in source:
                    output = self.run(
                        "docker",
                        "compose",
                        "-f",
                        source["private"],
                        "config",
                        "--hash",
                        service,
                    )
                else:
                    original = self.env
                    if source["application"]:
                        self.env = {**original, **self._application}
                    try:
                        output = self.compose(
                            Path(source["runtime"]),
                            source["sha"],
                            source["sha"],
                            source["deployed"],
                            "config",
                            "--hash",
                            service,
                        )
                    finally:
                        self.env = original
            fields = output.decode().split()
            require(
                len(fields) == 2
                and fields[0] == service
                and item["Config"]
                .get("Labels", {})
                .get("com.docker.compose.config-hash")
                == fields[1],
                "current Compose configuration drift: " + service,
            )
            actual_mounts = {m["Destination"]: m for m in item.get("Mounts", [])}
            for mount in spec.get("volumes", []):
                if mount.get("type") == "bind":
                    require(
                        mount["target"] in actual_mounts
                        and actual_mounts[mount["target"]].get("Source")
                        == mount["source"]
                        and bool(actual_mounts[mount["target"]].get("RW"))
                        == (not mount.get("read_only", False)),
                        "current private mount differs: " + service,
                    )

    def migrations(self, containers, before, baseline, prior, member):
        sources = {name: baseline for name in OWNERS}
        sources["account-portfolio"] = Path(member["runtime"])
        sources["food"] = Path(prior["runtime"])
        tag = containers["henukit-career-opportunities-1"]["Config"]["Image"].split(
            ":"
        )[-1]
        sources["career"] = self.path("/opt/henukit-releases") / tag
        sources["quizcraft"] = Path(prior["runtime"])
        result = {}
        for owner, runtime in sources.items():

            def inventory(root):
                folder = root / "migrations" / owner
                self.trusted_directory(folder)
                entries = {}
                for path in folder.iterdir():
                    self.trusted(path)
                    entries[path.name] = engine.digest(path)
                require(entries, "owner migration inventory is empty: " + owner)
                return entries

            old, new = inventory(runtime), inventory(self.runtime)
            require(
                old == new, "first stack transition prohibits schema changes: " + owner
            )
            result[owner] = {"runtime": str(runtime), "files": old}
        return result

    def preserve_configuration(self, before, after, containers):
        # The first adoption cannot redirect an owner to an unbacked database,
        # change Redis job/nonce authorities, or move writable user data.
        for service in SCOPE:
            previous = self.container_environment(
                containers["henukit-" + service + "-1"]
            )
            candidate = after["services"][service].get("environment", {})
            for key in set(previous) | set(candidate):
                if key.endswith(("DATABASE_URL", "REDIS_URL")):
                    require(
                        str(candidate.get(key, "")) == previous.get(key, ""),
                        "first stack transition prohibits owner data-route changes: "
                        + service
                        + "/"
                        + key,
                    )

            def writable(spec):
                return [
                    m for m in spec.get("volumes", []) if not m.get("read_only", False)
                ]

            require(
                writable(before["services"][service])
                == writable(after["services"][service]),
                "first stack transition prohibits writable data-mount changes: "
                + service,
            )
        for name in ("postgres", "redis"):
            previous = copy.deepcopy(before["services"][name])
            candidate = copy.deepcopy(after["services"][name])
            old_mounts = {m.get("target"): m for m in previous.get("volumes", [])}
            for mount in candidate.get("volumes", []):
                old = old_mounts.get(mount.get("target"), {})
                if (
                    old.get("type") == mount.get("type") == "bind"
                    and old.get("read_only") is mount.get("read_only") is True
                    and old.get("source") != mount.get("source")
                ):
                    for path in (Path(old["source"]), Path(mount["source"])):
                        self.trusted(path)
                    require(
                        engine.digest(Path(old["source"]))
                        == engine.digest(Path(mount["source"])),
                        "persistent service read-only initialization content changed",
                    )
                    mount["source"] = old["source"]
            require(
                previous == candidate,
                "persistent service configuration changed: " + name,
            )
        require(
            before["services"]["nginx"]["image"] == after["services"]["nginx"]["image"],
            "Nginx retained official image changed",
        )
        for key in membership.BOT_KEYS:
            old = (
                before["services"]["account-portfolio"].get("environment", {}).get(key)
            )
            require(
                old
                and after["services"]["account-portfolio"]
                .get("environment", {})
                .get(key)
                == old,
                "membership Bot credential would change",
            )
        for name, keys in {
            "console-gateway": ("ENABLED", "CLIENT_ID", "API_URL", "KEY_ID", "SECRET"),
            "quizcraft": ("ENABLED", "CLIENT_ID", "KEYS_JSON"),
        }.items():
            for suffix in keys:
                key = "QUIZCRAFT_CONSOLE_" + suffix
                require(
                    before["services"][name].get("environment", {}).get(key)
                    == self._application[key]
                    and after["services"][name].get("environment", {}).get(key)
                    == self._application[key],
                    "Practice application credential would change",
                )
        require(
            after["services"]["career-opportunities"]
            .get("environment", {})
            .get("CAREER_GETWORK_ENABLED")
            == "0",
            "candidate does not explicitly disable getWork scans",
        )

        def nginx_file(config):
            mounts = [
                m
                for m in config["services"]["nginx"].get("volumes", [])
                if m.get("target") == "/etc/nginx/conf.d/default.conf"
            ]
            require(
                len(mounts) == 1 and mounts[0].get("read_only") is True,
                "Nginx route mount is not fixed and read-only",
            )
            path = Path(mounts[0]["source"])
            self.trusted(path)
            return path.read_text()

        old, new = nginx_file(before), nginx_file(after)
        require(membership.ROUTE in new, "candidate omits membership Bot route")
        for route in re.findall(r"^\s*location\s+([^\n{]+)\s*\{", old, re.M):
            require(route.strip() in new, "candidate omits a retained Nginx route")

    def extra_tool(self, name):
        path = self.prefix / "bin" / name if self.testing else Path("/usr/bin") / name
        self.trusted(path)
        self.tools[name] = str(path)

    def technical_gates(self, baseline, check_gateway=True):
        require(
            self.run(
                "gh", "api", "repos/" + engine.REPO + "/issues/166", "--jq", ".state"
            )
            .decode()
            .strip()
            == "closed",
            "blocker issue #166 must be closed before full deployment",
        )
        account = self.runtime / "release-gates/account-production-boundary.env"
        self.trusted(account)
        expected = "\n".join(
            (
                "release_sha=" + self.o.sha,
                "status=pass",
                "account_console_mock_sources=absent",
                "account_transitive_mock_sources=absent",
                "account_payment_provider=easypay_or_disabled",
                "portal_require_gateway=1",
                "portal_allow_mock=0",
                "portal_api_default_mode=live",
            )
        )
        require(
            account.read_text().replace("\r", "").strip() == expected,
            "Account production boundary did not pass for the candidate",
        )
        oauth = self.runtime / "release-gates/oauth-continuation.env"
        artifact = self.artifacts / ("oauth-continuation-" + self.o.sha + ".env")
        self.trusted(oauth)
        self.trusted(artifact)
        tree = (
            self.run(
                "gh",
                "api",
                "repos/" + engine.REPO + "/git/commits/" + self.o.sha,
                "--jq",
                ".tree.sha",
            )
            .decode()
            .strip()
        )
        require(
            re.fullmatch(r"[0-9a-f]{40}", tree),
            "candidate source tree identity is missing",
        )
        canonical = (
            "format=henukit-oauth-continuation-gate-v1\nrelease_sha="
            + self.o.sha
            + "\nsource_tree="
            + tree
            + "\nresult=pass\n"
        )
        require(
            oauth.read_text() == artifact.read_text() == canonical,
            "workflow OAuth continuation evidence is missing or differs",
        )
        # Execute the already attested watcher policy at its pure read-only
        # boundary. No second implementation of its AI/payment/mock rules.
        watcher = (self.runtime / "bin/watch-henukit-actions.sh").read_text()
        start = "environment_value() {\n"
        end = "verify_account_boundary_manifest() {\n"
        require(
            watcher.count(start) == watcher.count(end) == 1,
            "watcher environment gate boundary is not uniquely defined",
        )
        block = watcher[watcher.index(start) : watcher.index(end)]
        require(
            block.count("verify_production_data_boundary() {\n") == 1,
            "watcher production gate is missing",
        )
        require(
            len(re.findall(r"^\}$", block, re.M)) == 2
            and not re.search(r"^verify_production_data_boundary\s*$", block, re.M),
            "watcher read-only gate function layout changed",
        )
        block = block.replace(start, "baseline_environment_value() {\n", 1)
        shell = (
            'set -Eeuo pipefail\ndie(){ printf "%s\\n" "$*" >&2; exit 1; }\nenv_file="$1"\n'
            + block
            + '\nenvironment_value(){ if [[ "$1" == CAREER_GETWORK_ENABLED ]]; then printf 0; else baseline_environment_value "$1"; fi; }\nverify_production_data_boundary\n'
        )
        self.extra_tool("bash")
        result = subprocess.run(
            [self.tools["bash"], "-s", "--", str(self.envfile)],
            input=shell.encode(),
            env=self.env,
            capture_output=True,
            timeout=60,
        )
        require(
            result.returncode == 0,
            "existing watcher production data boundary gate failed",
        )
        policy = self.path("/etc/henukit/actions-watch.env")
        self.trusted(policy, private=True)
        roles = re.findall(
            r"^HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE=([^\r\n]+)$", policy.read_text(), re.M
        )
        require(
            len(roles) == 1, "existing Account operator role is missing or duplicated"
        )
        role = roles[0].strip().strip("'\"")
        require(
            re.fullmatch(r"[a-z][a-z0-9-]{1,63}", role),
            "existing Account operator role is invalid",
        )
        permissions = (
            "account.tickets.read",
            "account.tickets.reply",
            "account.tickets.transition",
            "account.membership.write",
            "account.points.adjust",
            "account.orders.read",
            "account.orders.close",
            "account.orders.refund",
        )
        literals = ",".join("'" + p + "'" for p in permissions)
        sql = (
            "WITH selected_role AS (SELECT id FROM authorization_roles WHERE status='active' AND code='"
            + role
            + "'), required(code) AS (SELECT unnest(ARRAY["
            + literals
            + "]::text[])) SELECT json_build_object('roles',(SELECT count(*) FROM selected_role),'permissions',(SELECT count(*) FROM permission_codes p JOIN required r USING(code) WHERE p.status='active'),'granted',(SELECT count(DISTINCT rp.permission_code) FROM role_permissions rp JOIN selected_role sr ON sr.id=rp.role_id JOIN required r ON r.code=rp.permission_code),'audited',EXISTS(SELECT 1 FROM account_operator_role_grant_audit_events a JOIN selected_role sr ON sr.id=a.role_id WHERE a.role_code='"
            + role
            + "' AND a.permission_codes @> ARRAY["
            + literals
            + "]::text[]))"
        )
        output = self.run(
            "docker",
            "exec",
            "henukit-postgres-1",
            "sh",
            "-ceu",
            'psql -U "$POSTGRES_USER" -d platform -Atqc "$1"',
            "sh",
            sql,
        )
        require(
            json.loads(output)
            == {"roles": 1, "permissions": 8, "granted": 8, "audited": True},
            "existing Account operator grant/audit is incomplete; transition never creates grants",
        )
        gateway = self.gateway_gate(baseline, check_gateway)
        return {
            "oauth_source_tree": tree,
            "account_manifest_sha256": engine.digest(account),
            "policy_sha256": engine.digest(policy),
            "operator_role": role,
            "gateway_patch_manifest_sha256": gateway,
        }

    def gateway_gate(self, baseline, check):
        names = [
            "bin/deploy-epay-gateway-patches.sh",
            *(
                "infra/epay-gateway/patches/" + n
                for n in (
                    "0001-henukit-query-and-notify-outbox.patch",
                    "0002-henukit-close-refund-and-response-verification.patch",
                    "0003-henukit-private-checkout-handle.patch",
                )
            ),
        ]
        for name in names:
            self.trusted(self.runtime / name)
            self.trusted(baseline / name)
            require(
                engine.digest(self.runtime / name) == engine.digest(baseline / name),
                "first stack transition prohibits EasyPay gateway code changes",
            )
        manifest = "".join(
            engine.digest(self.runtime / name) + "  " + Path(name).name + "\n"
            for name in names[1:]
        )

        def environment_value(key):
            values = re.findall(
                r"^\s*" + re.escape(key) + r"\s*=(.*)$", self.envfile.read_text(), re.M
            )
            require(
                len(values) == 1,
                "existing EasyPay tenant configuration is missing or duplicated",
            )
            value = values[0].strip()
            if len(value) >= 2 and value[0] == value[-1] and value[0] in "'\"":
                value = value[1:-1]
            return value

        tenant_hash = hashlib.sha256(
            (
                environment_value("ACCOUNT_PORTFOLIO_EASYPAY_PID")
                + "\n"
                + environment_value("ACCOUNT_PORTFOLIO_EASYPAY_KEY")
            ).encode()
        ).hexdigest()
        helper = (self.runtime / names[0]).read_text()
        start, end = "gateway_env_value() {\n", "henukit_enabled="
        require(
            helper.count(start) == 1 and helper.count(end) == 1,
            "existing EasyPay tenant verification boundary changed",
        )
        tenant_reader = helper[helper.index(start) : helper.index(end)]
        require(
            len(re.findall(r"^\}$", tenant_reader, re.M)) == 1,
            "existing EasyPay tenant reader layout changed",
        )
        self.extra_tool("ssh")
        args = ("-oBatchMode=yes", "-oStrictHostKeyChecking=yes", "root@metaview.top")
        guard = (
            "set -Eeuo pipefail\n"
            + "expected='"
            + manifest.rstrip("\n")
            + "'\n"
            + "gateway_dir=/root/epay-gateway\n"
            + tenant_reader
            + "expected_tenant_hash='"
            + tenant_hash
            + "'\n"
            + """marker=/root/epay-gateway/.henukit-patches.sha256
[[ -f "$marker" && ! -L "$marker" && "$(stat -c %u "$marker")" == 0 ]]
[[ "$(cat "$marker")" == "$expected" ]]
[[ -f "$gateway_dir/.env" && ! -L "$gateway_dir/.env" && "$(stat -c %u "$gateway_dir/.env")" == 0 && "$(stat -c %a "$gateway_dir/.env")" == 600 ]]
[[ "$(gateway_env_value HENUKIT_EPAY_ENABLED)" == 1 ]]
tenant_pid="$(gateway_env_value HENUKIT_EPAY_PID)"
tenant_key="$(gateway_env_value HENUKIT_EPAY_KEY)"
[[ "$(printf '%s\\n%s' "$tenant_pid" "$tenant_key" | sha256sum | awk '{print $1}')" == "$expected_tenant_hash" ]]
notify_urls="$(gateway_env_value HENUKIT_EPAY_NOTIFY_URLS)"
return_origins="$(gateway_env_value HENUKIT_EPAY_RETURN_ORIGINS)"
[[ ",$notify_urls," == *,https://henukit.cn/api/v1/payment-providers/easypay/notifications,* ]]
[[ ",$return_origins," == *,https://henukit.cn,* ]]
systemctl is-active --quiet epay-gateway.service
curl --noproxy '*' --fail --silent --show-error --max-time 10 http://127.0.0.1:9219/health >/dev/null
printf gateway_ready
"""
        )
        require(
            self.run("ssh", *args, "bash", "-s", input=guard.encode()).decode().strip()
            == "gateway_ready",
            "existing MetaView gateway patch/health proof failed",
        )
        if check:
            staging = (
                self.run(
                    "ssh", *args, "mktemp -d /tmp/henukit-transition-check.XXXXXXXX"
                )
                .decode()
                .strip()
            )
            require(
                re.fullmatch(r"/tmp/henukit-transition-check\.[A-Za-z0-9]{8}", staging),
                "gateway returned an unsafe verification staging path",
            )
            try:
                stream = io.BytesIO()
                with tarfile.open(fileobj=stream, mode="w:gz") as archive:
                    for name in names:
                        archive.add(self.runtime / name, arcname=name, recursive=False)
                self.run(
                    "ssh", *args, "tar -xzf - -C " + staging, input=stream.getvalue()
                )
                self.run(
                    "ssh",
                    *args,
                    staging
                    + "/bin/deploy-epay-gateway-patches.sh /root/epay-gateway "
                    + staging
                    + "/infra/epay-gateway/patches --check",
                    timeout=600,
                )
            finally:
                self.run("ssh", *args, "rm -rf -- " + staging)
        return hashlib.sha256(manifest.encode()).hexdigest()

    def materials_payload(self, runtime):
        """Read the complete checksum inventory and fixed installer target map."""
        installer = runtime / "materials-runtime/install.sh"
        self.trusted(installer)
        payload = installer.parent
        manifest = payload / "SHA256SUMS"
        self.trusted(manifest)
        checksums = {}
        for line in manifest.read_text().splitlines():
            match = re.fullmatch(r"([0-9a-f]{64})  ([^\s]+)", line)
            require(
                match and match[2] not in checksums,
                "invalid materials checksum manifest",
            )
            checksums[match[2]] = match[1]
        actual = {
            str(p.relative_to(payload)): engine.digest(p)
            for p in payload.rglob("*")
            if p.is_file() and p != manifest
        }
        require(
            actual == checksums, "materials payload differs from its checksum manifest"
        )
        table = re.search(
            r"declare -a install_records=\(\n(.*?)\n\)", installer.read_text(), re.S
        )
        require(table, "materials installer has no fixed target table")
        records = []
        for line in table[1].splitlines():
            match = re.fullmatch(r'\s*"([^|"\s]+)\|(/[^|"\s]+)\|(0[0-7]{3})"', line)
            require(match, "unreviewed materials installer target table")
            source, target, mode = match.groups()
            require(
                not Path(source).is_absolute()
                and not any(p in ("..", ".") for p in Path(source).parts)
                and not any(p in ("..", ".") for p in Path(target).parts)
                and target.startswith(
                    (
                        "/usr/local/bin/henukit-",
                        "/usr/local/libexec/henukit/",
                        "/etc/systemd/system/henukit-",
                    )
                ),
                "materials target is outside constrained replay scope",
            )
            self.trusted(payload / source)
            records.append({"source": source, "target": target, "mode": int(mode, 8)})
        require(
            records and len({r["target"] for r in records}) == len(records),
            "duplicate materials installation targets",
        )
        return records, engine.digest(manifest), set(checksums)

    def file_identity(self, path):
        self.trusted(path)
        info = path.stat()
        return {
            "sha256": engine.digest(path),
            "mode": info.st_mode & 0o7777,
            "uid": info.st_uid,
            "gid": info.st_gid,
        }

    def historical_runtime(self, sha, runtime, proof, archive=None):
        """Authenticate historical bytes without importing or running old code."""
        self.trusted_directory(runtime)
        self.trusted_directory(proof)
        marker = runtime / "RELEASE_SHA"
        self.trusted(marker)
        require(
            marker.read_text().strip() == sha, "materials baseline runtime SHA differs"
        )
        archive = archive or proof / ("henukit-runtime-" + sha + ".tar.gz")
        sidecar = Path(str(archive) + ".sha256")
        manifest = proof / ("henukit-getwork-actions-" + sha + ".manifest")
        bundle = proof / ("henukit-getwork-actions-" + sha + ".attestation.json")
        trusted_root = proof / "trusted_root.jsonl"
        files = (archive, sidecar, manifest, bundle, trusted_root)
        for path in files:
            self.trusted(path)
        proof_identities = {str(path): self.file_identity(path) for path in files}
        require(
            sidecar.read_text().split() == [engine.digest(archive), archive.name],
            "materials baseline archive checksum mismatch",
        )
        lines = manifest.read_text().splitlines()
        headers = [
            "format=henukit-getwork-actions-release-v1",
            "release_sha=" + sha,
            "source_repository=" + engine.REPO,
            "source_ref=refs/heads/main",
            "signer_workflow=.github/workflows/deploy-henukit.yml",
            "builder_platform=linux/amd64",
        ]
        require(
            len(lines) == 8
            and lines[:6] == headers
            and re.fullmatch(
                r"artifact_sha256=[0-9a-f]{64}  henukit-getwork-mcp-"
                + sha
                + r"\.docker\.tar\.gz",
                lines[6],
            )
            and lines[7]
            == "artifact_sha256=" + engine.digest(archive) + "  " + archive.name,
            "materials baseline v1 manifest does not bind its historical runtime",
        )
        verified = json.loads(
            self.run(
                "gh",
                "attestation",
                "verify",
                manifest,
                "--repo",
                engine.REPO,
                "--bundle",
                bundle,
                "--signer-workflow",
                engine.REPO + "/.github/workflows/deploy-henukit.yml",
                "--source-ref",
                "refs/heads/main",
                "--source-digest",
                sha,
                "--predicate-type",
                "https://github.com/jry21223/HENU-Kit-DEV/attestations/getwork-actions-release-v1",
                "--deny-self-hosted-runners",
                "--custom-trusted-root",
                trusted_root,
                "--format",
                "json",
            )
        )
        require(
            isinstance(verified, list) and len(verified) == 1,
            "historical materials attestation must verify exactly one signer",
        )
        certificate = (
            verified[0]
            .get("verificationResult", {})
            .get("signature", {})
            .get("certificate", {})
        )
        require(
            certificate.get("sourceRepositoryDigest")
            == certificate.get("buildConfigDigest")
            == sha
            and certificate.get("sourceRepositoryRef") == "refs/heads/main"
            and certificate.get("runnerEnvironment") == "github-hosted"
            and certificate.get("buildConfigURI")
            == "https://github.com/"
            + engine.REPO
            + "/.github/workflows/deploy-henukit.yml@refs/heads/main",
            "historical materials signer/source identity differs",
        )
        invocation = certificate.get("runInvocationURI", "")
        source = re.fullmatch(
            r"https://github\.com/"
            + re.escape(engine.REPO)
            + r"/actions/runs/([1-9][0-9]*)/attempts/([1-9][0-9]*)",
            invocation,
        )
        require(source, "verified historical materials invocation is missing")
        run_id, attempt = map(int, source.groups())
        run = json.loads(
            self.run(
                "gh",
                "api",
                "repos/"
                + engine.REPO
                + "/actions/runs/"
                + str(run_id)
                + "/attempts/"
                + str(attempt),
            )
        )
        require(
            all(
                run.get(key) == value
                for key, value in {
                    "id": run_id,
                    "run_attempt": attempt,
                    "head_sha": sha,
                    "head_branch": "main",
                    "event": "push",
                    "status": "completed",
                    "conclusion": "success",
                    "path": ".github/workflows/deploy-henukit.yml",
                }.items()
            ),
            "historical materials workflow attempt did not complete successfully on main",
        )
        ancestry = json.loads(
            self.run(
                "gh",
                "api",
                "repos/" + engine.REPO + "/compare/" + sha + "..." + self.o.sha,
            )
        )
        require(
            ancestry.get("status") in ("ahead", "identical")
            and ancestry.get("behind_by") == 0
            and ancestry.get("base_commit", {}).get("sha")
            == ancestry.get("merge_base_commit", {}).get("sha")
            == sha,
            "historical materials source is not a merged-main ancestor",
        )
        inventory = self.runtime_inventory(runtime)
        with self.safe_tar(archive) as packaged:
            names = {
                member.name.removeprefix("./")
                for member in packaged.getmembers()
                if member.isfile()
            }
            require(
                names == set(inventory),
                "historical materials runtime inventory differs from its attested archive",
            )
            for name, checksum in inventory.items():
                raw = packaged.extractfile("./" + name).read()
                require(
                    hashlib.sha256(raw).hexdigest() == checksum,
                    "historical materials runtime bytes differ from its attested archive",
                )
        runtime_identities = {
            name: self.file_identity(runtime / name) for name in inventory
        }
        require(
            all(
                identity["sha256"] == inventory[name]
                for name, identity in runtime_identities.items()
            ),
            "historical materials runtime changed during verification",
        )
        require(
            all(
                self.file_identity(Path(name)) == identity
                for name, identity in proof_identities.items()
            ),
            "historical materials proof changed during verification",
        )
        return {
            "sha": sha,
            "runtime": str(runtime),
            "proof_dir": str(proof),
            "archive": str(archive),
            "archive_sha256": engine.digest(archive),
            "run_id": run_id,
            "attempt": attempt,
            "invocation": invocation,
            "proof_files": proof_identities,
            "runtime_files": runtime_identities,
        }

    def attested_materials_baseline(self, candidate_records):
        runtime = Path(self.o.materials_baseline_runtime)
        authority = self.historical_runtime(
            self.o.materials_baseline_sha,
            runtime,
            Path(self.o.materials_baseline_proof_dir),
        )
        records, payload_manifest, payload_files = self.materials_payload(runtime)
        require(
            {r["target"]: r for r in records}
            == {r["target"]: r for r in candidate_records},
            "historical/candidate materials installer target maps differ",
        )
        require(
            payload_files == {"install.sh", *(r["source"] for r in records)},
            "historical materials payload contains an unknown installation inventory",
        )
        return runtime, {
            **authority,
            "kind": "attested-retained",
            "payload_manifest_sha256": payload_manifest,
            "target_map": records,
        }

    def materials_baseline_unchanged(self, data):
        if data["kind"] == "full-release":
            return
        require(
            data["kind"] == "attested-retained", "unknown materials baseline authority"
        )
        runtime = Path(data["runtime"])
        require(
            set(self.runtime_inventory(runtime)) == set(data["runtime_files"]),
            "retained materials baseline inventory changed",
        )
        for name, expected in data["runtime_files"].items():
            require(
                self.file_identity(runtime / name) == expected,
                "retained materials baseline runtime changed",
            )
        for name, expected in data["proof_files"].items():
            require(
                self.file_identity(Path(name)) == expected,
                "retained materials baseline proof changed",
            )

    def materials_preflight(self, baseline):
        config = self.path("/etc/henukit-deploy/materials-activate.env")
        self.trusted(config, private=True)
        require(
            not re.search(
                r"^HENUKIT_MATERIALS_(?:CONVERTER|IMPORTER|PSQL|PG_SERVICE_FILE|PG_SERVICE|LEGACY_INVENTORY)=",
                config.read_text(),
                re.M,
            ),
            "modern materials baseline required; legacy activation keys need a separate migration",
        )
        for name in (
            "/usr/local/libexec/henukit/convert-henukit-slides.py",
            "/usr/local/libexec/henukit/import-henukit-materials.mjs",
            "/etc/henukit-deploy/materials-postgresql.conf",
            "/etc/henukit-deploy/materials-legacy-inventory.json",
        ):
            path = self.path(name)
            require(
                not path.exists() and not path.is_symlink(),
                "modern materials baseline required; retired source still exists",
            )
        candidate_records, _, _ = self.materials_payload(self.runtime)
        if self.o.materials_baseline_sha:
            baseline, authority = self.attested_materials_baseline(candidate_records)
        else:
            authority = {
                "kind": "full-release",
                "sha": (baseline / "RELEASE_SHA").read_text().strip(),
                "runtime": str(baseline),
            }
        records = []
        for entry in candidate_records:
            source, target, mode = entry["source"], entry["target"], entry["mode"]
            current = self.path(target)
            old = baseline / "materials-runtime" / source
            new = self.runtime / "materials-runtime" / source
            for path in (current, old, new):
                self.trusted(path)
            require(
                engine.digest(current) == engine.digest(old),
                "installed materials runtime differs from full baseline",
            )
            info = current.stat()
            if authority["kind"] == "attested-retained":
                require(
                    info.st_mode & 0o7777 == mode
                    and info.st_uid == self.uid
                    and info.st_gid == os.getegid(),
                    "installed materials target mode/owner differs from the selected historical baseline",
                )
            records.append(
                {
                    "source": source,
                    "target": target,
                    "mode": mode,
                    "before_mode": current.stat().st_mode & 0o7777,
                    "before_uid": info.st_uid,
                    "before_gid": info.st_gid,
                    "before_sha256": engine.digest(current),
                    "after_sha256": engine.digest(new),
                }
            )
        require(
            records and len({r["target"] for r in records}) == len(records),
            "duplicate materials installation targets",
        )
        backup = self.path("/opt/henukit-materials/runtime-backups") / self.o.sha
        require(
            not backup.exists() and not backup.is_symlink(),
            "materials candidate backup already exists",
        )
        active = self.path("/opt/henukit-materials/admin-resync/active")
        require(
            not active.exists() and not active.is_symlink(),
            "materials administrative request is unresolved",
        )
        plane = self.control_plane()
        return {
            "baseline": authority,
            "targets": records,
            "control_plane": plane,
            "activation_config_sha256": engine.digest(config),
            "backup": str(backup),
        }

    def unit(self, unit, property):
        return (
            self.run("systemctl", "show", "-p", property, "--value", unit)
            .decode()
            .strip()
        )

    def control_plane(self, allow_running=False):
        for name in (
            "henukit-materials-webhook.service",
            "henukit-materials-runner.service",
            "henukit-materials-webhook.path",
        ):
            require(
                self.unit(name, "LoadState") == "loaded",
                "materials control plane unit is missing",
            )
        runner = self.unit("henukit-materials-runner.service", "ActiveState")
        require(
            self.unit("henukit-materials-webhook.service", "ActiveState") == "active"
            and runner
            in (
                ("inactive", "active", "activating") if allow_running else ("inactive",)
            ),
            "materials control plane is not ready",
        )
        enabled = subprocess.run(
            [self.tools["systemctl"], "is-enabled", "henukit-materials-webhook.path"],
            capture_output=True,
            env=self.env,
            timeout=60,
        )
        require(enabled.returncode in (0, 1), "materials path state could not be read")
        path = enabled.stdout.decode().strip()
        require(path in ("enabled", "disabled"), "materials path state is unreviewed")
        load = self.unit("henukit-deploy-webhook.service", "LoadState")
        deploy = "absent" if load == "not-found" else "active"
        require(
            deploy == "absent"
            or load == "loaded"
            and self.unit("henukit-deploy-webhook.service", "ActiveState") == "active",
            "deploy receiver is not ready",
        )
        return {"path": path, "deploy": deploy}

    def atomic(self, path, content, mode=0o600):
        self.require_mutable_recovery()
        require(path.parent.is_dir(), "durable target parent is missing")
        self.trusted_directory(path.parent)
        temporary = path.parent / (".transition-" + secrets.token_hex(12))
        fd = os.open(
            temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, mode
        )
        with os.fdopen(fd, "wb") as stream:
            os.fchmod(stream.fileno(), mode)
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        self.durable_directory(path.parent)

    def alias(self, path, target):
        self.require_mutable_recovery()
        temporary = path.parent / (".transition-alias-" + secrets.token_hex(12))
        temporary.symlink_to(target)
        os.replace(temporary, path)
        self.durable_directory(path.parent)

    def checkpoint(self, data, phase):
        self.require_mutable_recovery()
        # Mutable state uses a unique, fsynced temporary. A killed writer can
        # leave an orphan without blocking the next owned recovery attempt.
        data["status"] = phase
        data["updated_at"] = self.timestamp()
        content = json.dumps(data, sort_keys=True).encode()
        self.atomic(self.record, content)
        audit = self.record.parent / (phase + "-" + secrets.token_hex(12) + ".json")
        descriptor = os.open(
            audit, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o400
        )
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        self.durable_directory(self.record.parent)

    def private_config(self, name, value):
        path = self.record.parent / name
        self.atomic(
            path,
            json.dumps(
                membership.Release.literal_config(value), sort_keys=True
            ).encode(),
        )
        return path

    def still_before(self, data, verify_source=True):
        self.verify_alias_authority(data)
        require(self.alias_snapshot(self.path("/opt/henukit/current")) == data["full_alias_before"], "full alias changed before activation")
        if verify_source:
            self.fresh_source(data)
        self.guard.pending()
        require(
            engine.digest(self.envfile) == data["environment_sha256"],
            "production environment changed",
        )
        runtime, sha = self.guard.full_baseline()
        require(
            str(runtime) == data["baseline_runtime"] and sha == data["baseline"],
            "full baseline changed",
        )
        require(
            self.marker.is_symlink()
            and str(self.marker.resolve()) == data["marker_before"],
            "scoped marker changed",
        )
        require(
            self.snapshot(self.containers(), True) == data["selected"]
            and self.snapshot(self.containers()) == data["unselected"],
            "mixed container checkpoint changed",
        )
        self.retained_unchanged(data)
        if verify_source:
            require(
                self.technical_gates(
                    Path(data["baseline_runtime"]), check_gateway=False
                )
                == data["technical_gates"],
                "production technical gate authority changed",
            )

    def fresh_source(self, data):
        require(
            self.source() == data["attempt"],
            "Actions attempt changed during transition preparation",
        )

    def retained_unchanged(self, data):
        if data.get("recovered_transition"):
            for name in ("target", "successor"):
                self.materials_baseline_unchanged(
                    {
                        **data["recovered_transition"][name],
                        "kind": "attested-retained",
                    }
                )
        self.materials_baseline_unchanged(data["materials"]["baseline"])
        self.materials_owned(data)
        for name, checksum in data["retained_files"].items():
            path = Path(name)
            self.trusted(path, private=True)
            require(
                engine.digest(path) == checksum,
                "retained private checkpoint configuration changed",
            )
        for name, checksum in data["retained_receipts"].items():
            path = Path(name)
            self.trusted(path, private=True)
            require(engine.digest(path) == checksum, "immutable scoped receipt changed")
        path = Path(data["application_file"])
        self.trusted(path, private=True)
        require(
            engine.digest(path) == data["application_secret_sha256"],
            "retained Practice credential changed",
        )
        for name, inventory in data["retained_runtimes"].items():
            require(
                self.runtime_inventory(Path(name)) == inventory,
                "retained mixed runtime changed",
            )

    def database_targets(self, containers):
        postgres = self.container_environment(containers["henukit-postgres-1"])
        result = {}
        for owner, (service, key, database) in OWNERS.items():
            env = self.container_environment(containers["henukit-" + service + "-1"])
            if owner == "portal" and not env.get(key):
                # The production Portal API explicitly supports Study as its
                # source until a dedicated Portal URL is configured.
                key, database = "STUDY_DATABASE_URL", "study"
            url = urlsplit(env.get(key, ""))
            require(
                url.scheme in ("postgres", "postgresql")
                and url.hostname == "postgres"
                and url.port in (None, 5432)
                and unquote(url.path) == "/" + database
                and unquote(url.username or "") == postgres.get("POSTGRES_USER")
                and unquote(url.password or "") == postgres.get("POSTGRES_PASSWORD")
                and not url.fragment
                and parse_qsl(url.query, keep_blank_values=True)
                in ([], [("sslmode", "disable")]),
                "owner backup target is not the existing local database: " + owner,
            )
            result[owner] = database
        return result

    def backup(self, data):
        self.require_mutable_recovery()
        self.atomic(
            self.record.parent / "configuration.backup", self.envfile.read_bytes()
        )
        require(
            engine.digest(self.record.parent / "configuration.backup")
            == data["environment_sha256"],
            "environment changed during backup",
        )
        evidence = {}
        for owner, database in self.database_targets(self.containers()).items():
            dump = self.run(
                "docker",
                "exec",
                "henukit-postgres-1",
                "sh",
                "-ceu",
                'pg_dump -U "$POSTGRES_USER" -d "$1" -Fc',
                "sh",
                database,
                timeout=180,
            )
            require(dump, "empty owner backup: " + owner)
            archive = self.record.parent / (owner + ".dump")
            self.atomic(archive, dump)
            verify = "henukit_transition_" + secrets.token_hex(12)
            self.run(
                "docker",
                "exec",
                "henukit-postgres-1",
                "sh",
                "-ceu",
                'createdb -U "$POSTGRES_USER" -T template0 "$1"',
                "sh",
                verify,
            )
            try:
                self.run(
                    "docker",
                    "exec",
                    "-i",
                    "henukit-postgres-1",
                    "sh",
                    "-ceu",
                    'pg_restore --exit-on-error --no-owner --no-privileges -U "$POSTGRES_USER" -d "$1"',
                    "sh",
                    verify,
                    input=dump,
                    timeout=180,
                )
                count = (
                    self.run(
                        "docker",
                        "exec",
                        "henukit-postgres-1",
                        "sh",
                        "-ceu",
                        'psql -U "$POSTGRES_USER" -d "$1" -Atqc "SELECT count(*) FROM information_schema.tables WHERE table_schema=\'public\'"',
                        "sh",
                        verify,
                    )
                    .decode()
                    .strip()
                )
                require(
                    count.isdigit() and int(count) > 0,
                    "isolated owner restore has no schema: " + owner,
                )
            finally:
                self.run(
                    "docker",
                    "exec",
                    "henukit-postgres-1",
                    "sh",
                    "-ceu",
                    'dropdb -U "$POSTGRES_USER" "$1"',
                    "sh",
                    verify,
                )
            evidence[owner] = {
                "sha256": engine.digest(archive),
                "bytes": archive.stat().st_size,
                "tables": int(count),
            }
        # Keep the existing host-PG16 owner route, namespace identity, exported
        # snapshot, and isolated restore proof. Do not regenerate its HMAC key.
        helper = self.practice_helper
        helper.database_target(self.containers())
        require(
            helper.database_identity == data["quizcraft_identity"],
            "QuizCraft owner route changed before backup",
        )
        archive = self.record.parent / "quizcraft.dump"
        with helper.database_snapshot() as snapshot:
            helper.run(
                "pg_dump",
                "--format=custom",
                "--snapshot",
                snapshot["snapshot"],
                "--file",
                archive,
                env=helper.pg_environment(helper.database_url),
                timeout=180,
            )
        self.trusted(archive, private=True)
        require(
            archive.stat().st_size > 0 and helper.run("pg_restore", "--list", archive),
            "QuizCraft backup is unreadable",
        )
        verify = "henukit_transition_" + secrets.token_hex(12)
        helper.run(
            "runuser",
            "-u",
            "postgres",
            "--",
            helper.tools["createdb"],
            "--template",
            "template0",
            "--",
            verify,
            env=helper.pg_environment(),
        )
        try:
            with archive.open("rb") as stream:
                restored = subprocess.run(
                    [
                        helper.tools["runuser"],
                        "-u",
                        "postgres",
                        "--",
                        helper.tools["pg_restore"],
                        "--exit-on-error",
                        "--no-owner",
                        "--no-privileges",
                        "--dbname",
                        verify,
                    ],
                    env=helper.pg_environment(),
                    stdin=stream,
                    capture_output=True,
                    timeout=180,
                )
            require(restored.returncode == 0, "isolated QuizCraft restore failed")
            counts = helper.psql(practice.COUNTS, database=verify)
            require(
                counts == snapshot["counts"],
                "isolated QuizCraft snapshot counts differ",
            )
        finally:
            helper.run(
                "runuser",
                "-u",
                "postgres",
                "--",
                helper.tools["dropdb"],
                "--",
                verify,
                env=helper.pg_environment(),
            )
        with archive.open("rb") as stream:
            os.fsync(stream.fileno())
        evidence["quizcraft"] = {
            "sha256": engine.digest(archive),
            "bytes": archive.stat().st_size,
            "counts": counts,
        }
        data["database_backups"] = evidence
        self.durable_directory(self.record.parent)

    def candidate_environment(self, data):
        raw = self.envfile.read_bytes()
        require(
            engine.digest(self.envfile) == data["environment_sha256"],
            "environment changed before candidate preparation",
        )
        additions = {"CAREER_GETWORK_ENABLED": "0", **self._application}
        existing = {}
        for line in raw.decode().splitlines():
            match = re.fullmatch(r"([A-Z][A-Z0-9_]*)=(.*)", line)
            if match and match[1] in additions:
                require(
                    match[1] not in existing, "duplicate application environment key"
                )
                existing[match[1]] = match[2]
        # First transition appends dedicated keys only. It never rewrites a
        # pre-existing credential or silently changes a flag set to 1.
        for key, value in existing.items():
            require(
                value == additions[key],
                "existing transition environment key conflicts: " + key,
            )
        extra = "".join(
            k + "=" + v + "\n" for k, v in additions.items() if k not in existing
        )
        result = (
            raw + (b"\n" if raw and not raw.endswith(b"\n") else b"") + extra.encode()
        )
        path = self.record.parent / "candidate.env"
        self.atomic(path, result)
        data["candidate_environment_sha256"] = engine.digest(path)

    def restore_control_plane(self, expected):
        self.run("systemctl", "daemon-reload")
        self.run("systemctl", "restart", "henukit-materials-webhook.service")
        if expected["deploy"] == "active":
            self.run("systemctl", "restart", "henukit-deploy-webhook.service")
        operation = "enable" if expected["path"] == "enabled" else "disable"
        self.run("systemctl", operation, "--now", "henukit-materials-webhook.path")
        require(
            self.control_plane(allow_running=expected["path"] == "enabled") == expected,
            "materials control plane restoration failed",
        )

    def install_materials(self, data):
        self.retained_unchanged(data)
        state = data["materials"]
        require(
            self.control_plane() == state["control_plane"],
            "materials operational state changed",
        )
        require(
            engine.digest(self.path("/etc/henukit-deploy/materials-activate.env"))
            == state["activation_config_sha256"],
            "materials activation config changed",
        )
        self.materials_matches(data, False)
        root = Path(state["backup"])
        root.mkdir(parents=True, mode=0o700)
        self.trusted_directory(root)
        self.durable_directory(root.parent)
        data["materials_record_created"] = True
        self.checkpoint(data, "materials_installing")
        self.run("systemctl", "disable", "--now", "henukit-materials-webhook.path")
        require(
            self.unit("henukit-materials-runner.service", "ActiveState") == "inactive",
            "materials runner did not quiesce",
        )
        installer = self.runtime / "materials-runtime/install.sh"
        result = subprocess.run(
            [str(installer), "--release-sha", self.o.sha],
            env=self.env,
            capture_output=True,
            timeout=240,
        )
        require(result.returncode == 0, "candidate materials installation failed")
        self.materials_matches(data, True)
        require(
            self.control_plane() == {**state["control_plane"], "path": "disabled"},
            "materials installer reopened an automatic publication trigger",
        )

    def materials_matches(self, data, candidate):
        for target in data["materials"]["targets"]:
            path = self.path(target["target"])
            self.trusted(path)
            info = path.stat()
            require(
                info.st_uid == (self.uid if candidate else target["before_uid"])
                and info.st_gid
                == (os.getegid() if candidate else target["before_gid"]),
                "installed materials runtime owner differs",
            )
            require(
                engine.digest(path)
                == target["after_sha256" if candidate else "before_sha256"],
                "installed materials runtime identity differs",
            )
            require(
                path.stat().st_mode & 0o7777
                == target["mode" if candidate else "before_mode"],
                "installed materials runtime mode differs",
            )
        require(
            engine.digest(self.path("/etc/henukit-deploy/materials-activate.env"))
            == data["materials"]["activation_config_sha256"],
            "materials activation config drift",
        )

    @staticmethod
    def before_materials_identity(target):
        return {
            "sha256": target["before_sha256"],
            "mode": target["before_mode"],
            "uid": target["before_uid"],
            "gid": target["before_gid"],
        }

    def materials_recovery_record(self, data):
        root = Path(data["materials"]["backup"])
        targets = {target["target"]: target for target in data["materials"]["targets"]}
        backups = {}
        if root.exists() or root.is_symlink():
            self.trusted_directory(root)
            for path in sorted(root.rglob("*")):
                if path.is_dir() and not path.is_symlink():
                    self.trusted_directory(path)
                    continue
                name = "/" + str(path.relative_to(root))
                require(
                    name in targets
                    and self.file_identity(path)
                    == self.before_materials_identity(targets[name]),
                    "materials installer backup identity changed",
                )
                backups[name] = path
        else:
            require(
                not data.get("materials_record_created"),
                "materials installer backup record is missing",
            )
        return backups

    def materials_owned(self, data):
        # Interrupted installation may be a known B/candidate mixture. A later
        # independently installed C must never be adopted or overwritten.
        backups = self.materials_recovery_record(data)
        for target in data["materials"]["targets"]:
            current = self.file_identity(self.path(target["target"]))
            before = self.before_materials_identity(target)
            candidate = {
                "sha256": target["after_sha256"],
                "mode": target["mode"],
                "uid": self.uid,
                "gid": os.getegid(),
            }
            require(
                current in (before, candidate),
                "installed materials target drifted outside the owned transition",
            )
            require(
                current == before or target["target"] in backups,
                "changed materials target has no verified original backup",
            )

    @contextlib.contextmanager
    def materials_lock(self):
        path = self.path("/opt/henukit-materials/orchestration.lock")
        self.trusted_directory(path.parent)
        descriptor = os.open(path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        with os.fdopen(descriptor, "a+") as stream:
            self.trusted(path, private=True)
            fcntl.flock(stream, fcntl.LOCK_EX | fcntl.LOCK_NB)
            require(
                self.unit("henukit-materials-runner.service", "ActiveState")
                == "inactive",
                "materials publication is active during runtime replay",
            )
            yield

    def restore_materials(self, data):
        with self.materials_lock():
            self.replay_materials(data)

    def replay_materials(self, data):
        self.materials_owned(data)
        root = Path(data["materials"]["backup"])
        self.trusted_directory(root)
        targets = {t["target"]: t for t in data["materials"]["targets"]}
        copies = []
        for path in sorted(root.rglob("*")):
            if path.is_dir() and not path.is_symlink():
                self.trusted_directory(path)
                continue
            self.trusted(path)
            target = "/" + str(path.relative_to(root))
            require(
                target in targets
                and self.file_identity(path)
                == self.before_materials_identity(targets[target]),
                "materials installer record is not replayable",
            )
            copies.append(
                (self.path(target), path.read_bytes(), targets[target]["before_mode"])
            )
        # An interrupted install may have copied only a prefix. Every changed
        # target must have its own original backup before any replay begins.
        backed = {str(path) for path, _, _ in copies}
        for target in targets.values():
            path = self.path(target["target"])
            self.trusted(path)
            require(
                engine.digest(path) == target["before_sha256"] or str(path) in backed,
                "changed materials target has no installer backup",
            )
        for path, content, mode in copies:
            self.atomic(path, content, mode)
        self.materials_matches(data, False)

    def switch(self, filename):
        self.run(
            "docker",
            "compose",
            "-f",
            self.record.parent / filename,
            "up",
            "-d",
            "--pull",
            "never",
            "--no-build",
            "--no-deps",
            *SCOPE,
            timeout=240,
        )

    def loaded_images_match(self, data):
        for service in SCOPE:
            tag = data["after"]["services"][service]["image"]
            image = json.loads(self.run("docker", "image", "inspect", tag))[0]
            require(
                image["Id"] == data["images"][service]
                and image["Architecture"] == "amd64"
                and image["Os"] == "linux",
                "loaded candidate image was replaced before activation",
            )

    def health(self, data, candidate):
        config = data["after" if candidate else "before"]
        images = (
            data["images"]
            if candidate
            else {s: data["selected"]["henukit-" + s + "-1"]["image"] for s in SCOPE}
        )
        for _ in range(30):
            containers = self.containers()
            ready = True
            for service in SCOPE:
                item = containers["henukit-" + service + "-1"]
                running = (
                    True
                    if candidate
                    else data["selected"]["henukit-" + service + "-1"]["running"]
                )
                ready &= (
                    item["Image"] == images[service]
                    and item["Config"]["Image"] == config["services"][service]["image"]
                    and item["State"]["Running"] == running
                )
                if running:
                    ready &= (
                        item["State"].get("Health", {}).get("Status", "healthy")
                        == "healthy"
                    )
            if ready:
                break
            if not self.testing:
                time.sleep(2)
        else:
            raise engine.ReleaseError(
                "stack did not reach expected image and health state"
            )
        require(
            self.snapshot(containers) == data["unselected"],
            "persistent or excluded containers changed",
        )
        self.configuration_matches(
            containers,
            config,
            compose_file=self.record.parent
            / ("candidate.compose.json" if candidate else "mixed.compose.json"),
        )
        self.run("docker", "exec", "henukit-nginx-1", "nginx", "-t")
        self.public_acceptance(containers)
        if candidate:
            self.disabled_source(data)
        self.retained_unchanged(data)

    def public_acceptance(self, containers):
        for path in (
            "/",
            "/food",
            "/library",
            "/practice",
            "/career",
            "/api/v1/healthz",
            "/api/v1/library/materials",
            "/api/v1/practice/catalog",
        ):
            self.http(path, (200,))
        self.http("/api/v1/account/summary", (401, 403))
        self.workshop_authentication(containers)
        self.http("/api/v1/bot/membership-entitlement", (401, 403))
        self.http("/quiz/", (404,))
        self.http("/study-api/healthz", (404,))

    def workshop_authentication(self, containers):
        # Match the established Practice gate: Console owns the public route;
        # the /console/ path belongs only to the private QuizCraft owner.
        self.http(
            "/api/v1/quizcraft/workshop/catalog",
            (401,),
            origin="https://console.henukit.cn",
        )
        networks = containers["henukit-quizcraft-1"]["NetworkSettings"]["Networks"]
        addresses = {n.get("IPAddress") for n in networks.values()}
        require(len(addresses) == 1, "ambiguous QuizCraft owner endpoint")
        address = next(iter(addresses))
        require(
            ipaddress.ip_address(address).version == 4
            and ipaddress.ip_address(address).is_private,
            "unreviewed QuizCraft owner endpoint",
        )
        for _ in range(30):
            with tempfile.NamedTemporaryFile() as output:
                code = (
                    self.run(
                        "curl",
                        "--silent",
                        "--show-error",
                        "--noproxy",
                        "*",
                        "--max-time",
                        "10",
                        "--output",
                        output.name,
                        "--write-out",
                        "%{http_code}",
                        "http://" + address + ":10089/api/v1/console/workshop/catalog",
                    )
                    .decode()
                    .strip()
                )
            if code == "401":
                return
            if not self.testing:
                time.sleep(2)
        raise engine.ReleaseError("private Workshop authentication acceptance failed")

    def disabled_source(self, data):
        # Authenticate at the same signed owner boundary used by Portal. A 503
        # without the exact disabled-source body is not acceptance evidence.
        credentials = data["after"]["services"]["career-opportunities"]["environment"]
        item = self.containers()["henukit-career-opportunities-1"]
        networks = item.get("NetworkSettings", {}).get("Networks", {})
        addresses = {
            n.get("IPAddress") for n in networks.values() if n.get("IPAddress")
        }
        require(len(addresses) == 1, "Career private network address is ambiguous")
        address = next(iter(addresses))
        require(
            ipaddress.ip_address(address).is_private,
            "Career probe destination is not private",
        )
        path = "/api/v1/career/searches"
        body, actor = b"{}", "00000000-0000-4000-8000-000000000000"
        timestamp, nonce = str(int(time.time())), secrets.token_urlsafe(24)
        canonical = "\n".join(
            ("POST", path, timestamp, nonce, hashlib.sha256(body).hexdigest(), actor)
        )
        secret = credentials["CAREER_SERVICE_SECRET"]
        signature = (
            base64.urlsafe_b64encode(
                hmac.digest(secret.encode(), canonical.encode(), "sha256")
            )
            .decode()
            .rstrip("=")
        )
        headers = {
            "Authorization": "Basic "
            + base64.b64encode(
                (credentials["CAREER_SERVICE_CLIENT_ID"] + ":" + secret).encode()
            ).decode(),
            "X-Service-Id": credentials["CAREER_SERVICE_CLIENT_ID"],
            "X-Key-Id": credentials["CAREER_SERVICE_KEY_ID"],
            "X-Actor-User-Id": actor,
            "X-Timestamp": timestamp,
            "X-Nonce": nonce,
            "X-Signature": signature,
            "Content-Type": "application/json",
        }
        # Credentials travel through stdin to the trusted curl process, never
        # argv, receipt text, subprocess errors, or a secret-bearing URL.
        lines = ['request = "POST"', 'data = "{}"']
        for key, value in headers.items():
            require(
                not any(c in value for c in '\r\n"\\'),
                "invalid Career signing credential",
            )
            lines.append('header = "' + key + ": " + value + '"')
        with tempfile.NamedTemporaryFile() as response:
            output = self.run(
                "curl",
                "--silent",
                "--show-error",
                "--noproxy",
                "*",
                "--max-time",
                "10",
                "--config",
                "-",
                "--output",
                response.name,
                "--write-out",
                "%{http_code}",
                "http://" + address + ":8097" + path,
                input=("\n".join(lines) + "\n").encode(),
            )
            value = json.loads(Path(response.name).read_text())
        require(
            output.decode().strip() == "503"
            and value.get("error", {}).get("code") == "JOB_SOURCE_DISABLED",
            "Career disabled-source contract failed",
        )

    def validate(self, data):
        self.verify_alias_authority(data)
        if self.o.materials_baseline_sha:
            selected = data.get("materials", {}).get("baseline", {})
            require(
                selected.get("kind") == "attested-retained"
                and selected.get("sha") == self.o.materials_baseline_sha
                and selected.get("runtime") == self.o.materials_baseline_runtime
                and selected.get("proof_dir") == self.o.materials_baseline_proof_dir,
                "materials baseline selection differs from the durable transition",
            )
        require(
            data.get("status")
            in (
                "intent",
                "backed_up",
                "images_loaded",
                "materials_installing",
                "applying",
                "committing",
                "accepted",
                "active",
                "rolling_back",
                "rollback_accepted",
                "rolled_back",
            ),
            "unknown durable transition state",
        )
        if self.recovery:
            self.pinned_receipt(data)
            require(
                self.recovery_authority,
                "successor recovery authority has not been verified",
            )
            self.recovery_unchanged(data)
        else:
            require(
                data.get("format") == FORMAT
                and data.get("scope") == list(SCOPE)
                and data.get("sha") == self.o.sha
                and data.get("run_id") == self.o.run_id
                and data.get("helper_sha256") == engine.digest(self.helper_file)
                and data.get("runtime") == str(self.runtime),
                "transition receipt identity mismatch",
            )
        self.retained_unchanged(data)
        actual = self.runtime_inventory(Path(data["runtime"]))
        require(actual == data["runtime_hashes"], "transition retained runtime changed")
        for name, checksum in data["configuration_hashes"].items():
            path = self.record.parent / name
            self.trusted(path, private=True)
            require(
                engine.digest(path) == checksum,
                "transition private configuration changed",
            )

        require(
            self.snapshot(self.containers()) == data["unselected"],
            "persistent or excluded containers changed",
        )
        self.selected_owned(data)
        owned = (
            data.get("accepted_selected")
            if data["status"] in ("active", "accepted")
            else data.get("rollback_checkpoint")
            if data["status"] in ("rolled_back", "rollback_accepted")
            else None
        )
        if owned is not None:
            require(
                self.snapshot(self.containers(), True) == owned,
                "settled selected container identities changed",
            )
        require(
            engine.digest(self.envfile)
            in (data["environment_sha256"], data["candidate_environment_sha256"]),
            "production environment drifted outside the owned transition",
        )
        for path in (
            self.path("/opt/henukit/current"),
            self.path("/opt/henukit-current"),
        ):
            require(
                path.is_symlink()
                and path.lstat().st_uid == self.uid
                and str(path.resolve()) in (data["baseline_runtime"], data["runtime"]),
                "full alias drifted outside the owned transition",
            )
        last = self.watch / "last-activated-sha"
        self.trusted(last)
        require(
            last.read_text().strip() in (data["baseline"], data["sha"]),
            "last activated marker drifted",
        )
        if self.marker.is_symlink():
            require(
                self.marker.lstat().st_uid == self.uid
                and str(self.marker.resolve()) == data["marker_before"],
                "foreign scoped marker",
            )
        else:
            audit = self.record.parent / "scoped-marker.before"
            require(
                not self.marker.exists()
                and audit.is_symlink()
                and str(audit.resolve()) == data["marker_before"],
                "scoped marker vanished without a transition audit",
            )

    @staticmethod
    def transaction_identity(data):
        mutable = {"status", "updated_at", "rollback_checkpoint", "recovery_authority"}
        return hashlib.sha256(
            json.dumps(
                {k: v for k, v in data.items() if k not in mutable}, sort_keys=True
            ).encode()
        ).hexdigest()

    def pinned_receipt(self, data):
        require(
            data.get("format") == FORMAT
            and data.get("scope") == list(SCOPE)
            and data.get("sha") == FAILED_SHA
            and data.get("run_id") == FAILED_RUN
            and data.get("attempt") == 1
            and data.get("helper_sha256") == FAILED_HELPER
            and data.get("runtime")
            == str(self.path("/opt/henukit-releases") / FAILED_SHA)
            and data.get("status")
            in ("rolling_back", "rollback_accepted", "rolled_back"),
            "receipt is not the pinned failed transition rollback",
        )
        require(
            self.o.sha != FAILED_SHA, "failed candidate cannot recover or retry itself"
        )

    def recovery_unchanged(self, data):
        authority = self.recovery_authority
        require(
            self.transaction_identity(data) == authority["transaction_identity"],
            "original transition identity changed during recovery",
        )
        self.materials_baseline_unchanged(
            {**authority["target"], "kind": "attested-retained"}
        )
        self.materials_baseline_unchanged(
            {**authority["successor"], "kind": "attested-retained"}
        )

    def prepare_recovery(self, data, proof):
        self.pinned_receipt(data)
        attempt = self.source()
        self.provenance()
        successor = self.historical_runtime(
            self.o.sha,
            self.runtime,
            self.proof,
            self.archive("henukit-runtime-" + self.o.sha + ".tar.gz"),
        )
        require(
            str(successor["run_id"]) == self.o.run_id
            and successor["attempt"] == attempt,
            "successor attestation workflow invocation differs from current attempt",
        )
        archive = (
            self.path("/opt/henukit-staging")
            / FAILED_SHA
            / ("henukit-runtime-" + FAILED_SHA + ".tar.gz")
        )
        target = self.historical_runtime(
            FAILED_SHA, Path(data["runtime"]), proof, archive
        )
        require(
            str(target["run_id"]) == FAILED_RUN
            and target["attempt"] == 1
            and target["runtime_files"]["bin/" + self.helper_file.name]["sha256"]
            == FAILED_HELPER
            and {
                name: value["sha256"] for name, value in target["runtime_files"].items()
            }
            == data["runtime_hashes"],
            "pinned transition attestation identity differs",
        )
        authority = {
            "format": "henukit-stack-recovery-authority-v1",
            "sha": self.o.sha,
            "run_id": self.o.run_id,
            "attempt": attempt,
            "runtime": str(self.runtime),
            "helper_sha256": engine.digest(self.helper_file),
            "successor": successor,
            "target": target,
            "transaction_identity": self.transaction_identity(data),
        }
        self.recovery_authority = authority
        self.validate(data)
        self.workshop_authentication(self.containers())
        require(
            self.source() == attempt,
            "successor Actions attempt changed before recovery",
        )
        return authority

    def recovery_audit(self, data, authority):
        self.require_mutable_recovery()
        name = "recovery-" + self.o.sha + "-" + str(authority["attempt"]) + ".json"
        path = self.record.parent / name
        if path.exists():
            self.trusted(path, private=True)
            require(
                json.loads(path.read_text())["authority"] == authority,
                "existing recovery authority differs",
            )
        else:
            content = json.dumps(
                {
                    "authority": authority,
                    "original_receipt_sha256": engine.digest(self.record),
                },
                sort_keys=True,
            ).encode()
            incoming = path.parent / (".recovery-incoming-" + secrets.token_hex(12))
            fd = os.open(
                incoming, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o400
            )
            try:
                with os.fdopen(fd, "wb") as stream:
                    stream.write(content)
                    stream.flush()
                    os.fsync(stream.fileno())
                os.link(incoming, path, follow_symlinks=False)
                self.durable_directory(path.parent)
            finally:
                incoming.unlink(missing_ok=True)
            self.durable_directory(path.parent)
        audits = data.setdefault("recovery_authority", {})
        reference = {"file": name, "sha256": engine.digest(path)}
        require(
            self.o.sha not in audits or audits[self.o.sha] == reference,
            "recovery audit reference changed",
        )
        audits[self.o.sha] = reference
        self.checkpoint(data, data["status"])

    def selected_owned(self, data):
        containers = self.containers()
        for service in SCOPE:
            item = containers["henukit-" + service + "-1"]
            original = data["selected"]["henukit-" + service + "-1"]
            matches = []
            for candidate in (False, True):
                config = data["after" if candidate else "before"]["services"][service]
                image = data["images"].get(service) if candidate else original["image"]
                if (
                    image is None
                    or item["Image"] != image
                    or item["Config"]["Image"] != config["image"]
                ):
                    continue
                expected = (
                    data["candidate_environment"].get(
                        service, data["before_environment"][service]
                    )
                    if candidate
                    else data["before_environment"][service]
                )
                if self.container_environment(item) != expected:
                    continue
                mounts = {
                    m["Destination"]: (m["Source"], bool(m.get("RW")))
                    for m in item.get("Mounts", [])
                    if m.get("Type", "bind") == "bind"
                }
                expected_mounts = {
                    m["target"]: (m["source"], not m.get("read_only", False))
                    for m in config.get("volumes", [])
                    if m.get("type") == "bind"
                }
                if mounts != expected_mounts:
                    continue
                file = self.record.parent / (
                    "candidate.compose.json" if candidate else "mixed.compose.json"
                )
                hashed = (
                    self.run(
                        "docker", "compose", "-f", file, "config", "--hash", service
                    )
                    .decode()
                    .split()
                )
                if (
                    len(hashed) == 2
                    and hashed[0] == service
                    and hashed[1]
                    == item["Config"]
                    .get("Labels", {})
                    .get("com.docker.compose.config-hash")
                ):
                    matches.append(candidate)
            require(
                matches,
                "selected container drifted outside the owned transition: " + service,
            )

    def final_state(self, data, candidate):
        self.verify_alias_authority(data)
        expected = data["runtime" if candidate else "baseline_runtime"]
        for path in (
            self.path("/opt/henukit/current"),
            self.path("/opt/henukit-current"),
        ):
            require(str(path.resolve()) == expected, "committed full alias differs")
        require(
            (self.watch / "last-activated-sha").read_text().strip()
            == data["sha" if candidate else "baseline"],
            "committed full marker differs",
        )
        require(
            engine.digest(self.envfile)
            == data[
                "candidate_environment_sha256" if candidate else "environment_sha256"
            ],
            "committed environment differs",
        )
        require(
            (not self.marker.exists() and not self.marker.is_symlink())
            if candidate
            else self.marker.is_symlink()
            and str(self.marker.resolve()) == data["marker_before"],
            "committed scoped marker differs",
        )
        require(
            self.control_plane(
                allow_running=data["materials"]["control_plane"]["path"] == "enabled"
            )
            == data["materials"]["control_plane"],
            "committed materials operational state differs",
        )

    def rollback(self, data):
        self.require_mutable_recovery()
        self.verify_alias_authority(data)
        self.validate(data)
        status = data["status"]
        if status == "rolled_back":
            self.final_state(data, False)
            self.health(data, False)
            self.materials_matches(data, False)
            return "already_rolled_back"
        if status == "rollback_accepted":
            self.health(data, False)
            self.materials_matches(data, False)
            self.restore_control_plane(data["materials"]["control_plane"])
            self.final_state(data, False)
            self.checkpoint(data, "rolled_back")
            return "rolled_back"
        if status in ("intent", "backed_up", "images_loaded"):
            self.still_before(data, verify_source=False)
            self.materials_matches(data, False)
            data["rollback_checkpoint"] = self.snapshot(self.containers(), True)
            self.checkpoint(data, "rolled_back")
            return "rolled_back"
        self.checkpoint(data, "rolling_back")
        with self.materials_lock():
            return self.restore_mixed(data)

    def restore_mixed(self, data):
        self.require_mutable_recovery()
        self.materials_owned(data)
        self.rollback_images(data["selected"])
        self.run("systemctl", "disable", "--now", "henukit-materials-webhook.path")
        require(
            self.unit("henukit-materials-runner.service", "ActiveState") == "inactive",
            "materials publication is active during rollback",
        )
        self.atomic(
            self.envfile, (self.record.parent / "configuration.backup").read_bytes()
        )
        stopped = [
            s for s in SCOPE if not data["selected"]["henukit-" + s + "-1"]["running"]
        ]
        running = [service for service in SCOPE if service not in stopped]
        if stopped:
            self.run(
                "docker",
                "compose",
                "-f",
                self.record.parent / "mixed.compose.json",
                "stop",
                *stopped,
            )
        base = (
            "compose",
            "-f",
            self.record.parent / "mixed.compose.json",
            "up",
            "--pull",
            "never",
            "--no-build",
        )
        self.run("docker", *base, "-d", "--no-deps", *running, timeout=240)
        if stopped:
            # Old Career must be recreated without ever starting its enabled
            # job-source worker, even for an interval before a final stop.
            self.run(
                "docker",
                *base,
                "--no-start",
                "--force-recreate",
                "--no-deps",
                *stopped,
                timeout=240,
            )
        if Path(data["materials"]["backup"]).exists():
            self.replay_materials(data)
        else:
            self.materials_matches(data, False)
        self.health(data, False)
        self.publish_full_alias(data, data["baseline_runtime"], restoring=True)
        self.atomic(
            self.watch / "last-activated-sha", (data["baseline"] + "\n").encode()
        )
        self.alias(self.marker, data["marker_before"])
        data["rollback_checkpoint"] = self.snapshot(self.containers(), True)
        self.checkpoint(data, "rollback_accepted")
        self.restore_control_plane(data["materials"]["control_plane"])
        self.final_state(data, False)
        # Original receipts remain byte-identical. Recreated IDs are owned by
        # this durable rollback receipt, never rewritten into old checkpoints.
        self.checkpoint(data, "rolled_back")
        return "rolled_back"

    def execute(self):
        self.require_mutable_recovery()
        if self.recovery:
            self.trusted(self.record, private=True)
            data = json.loads(self.record.read_text())
            with self.lock_watcher():
                authority = self.prepare_recovery(data, Path(self.o.recovery_proof_dir))
                self.recovery_audit(data, authority)
                return self.rollback(data)
        if self.record.exists():
            self.trusted(self.record, private=True)
            data = json.loads(self.record.read_text())
            with self.lock_watcher():
                self.validate(data)
                if self.o.rollback:
                    return self.rollback(data)
                if data["status"] in ("active", "accepted"):
                    self.health(data, True)
                    self.materials_matches(data, True)
                    if data["status"] == "accepted":
                        self.restore_control_plane(data["materials"]["control_plane"])
                        self.final_state(data, True)
                        self.checkpoint(data, "active")
                        return "active"
                    self.final_state(data, True)
                    return "already_active"
                require(
                    data["status"] != "rolled_back",
                    "rolled-back transition needs a new audited candidate",
                )
                self.rollback(data)
                raise engine.ReleaseError(
                    "interrupted transition restored; inspect its durable receipt before retry"
                )
        require(not self.o.rollback, "no owned transition exists to roll back")
        data = self.preflight()
        with self.lock_watcher():
            self.still_before(data)
            self.record.parent.mkdir(parents=True, mode=0o700)
            self.trusted_directory(self.record.parent)
            self.private_config("mixed.compose.json", data["before"])
            self.private_config("candidate.compose.json", data["after"])
            self.candidate_environment(data)
            self.atomic(
                self.record.parent / "configuration.backup", self.envfile.read_bytes()
            )
            data["configuration_hashes"] = {
                name: engine.digest(self.record.parent / name)
                for name in (
                    "mixed.compose.json",
                    "candidate.compose.json",
                    "candidate.env",
                    "configuration.backup",
                )
            }
            self.checkpoint(data, "intent")
            try:
                self.backup(data)
                self.checkpoint(data, "backed_up")
                self.still_before(data)
                for service in IMAGES:
                    archive = self.archive(
                        "henukit-" + service + "-" + self.o.sha + ".docker.tar.gz"
                    )
                    self.run("docker", "load", "--input", archive, timeout=180)
                    image = json.loads(
                        self.run(
                            "docker",
                            "image",
                            "inspect",
                            "henukit-" + service + ":" + self.o.sha,
                        )
                    )[0]
                    require(
                        image["Id"] in data["image_candidates"][service]
                        and image["Architecture"] == "amd64"
                        and image["Os"] == "linux",
                        "loaded candidate image identity differs",
                    )
                    data["images"][service] = image["Id"]
                data["images"]["nginx"] = data["selected"]["henukit-nginx-1"]["image"]
                require(
                    shutil.disk_usage(self.artifacts).free >= 4096 * 1024 * 1024,
                    "less than 4096 MiB free after image loading",
                )
                self.checkpoint(data, "images_loaded")
                self.still_before(data)
                self.install_materials(data)
                self.checkpoint(data, "applying")
                self.still_before(data)
                self.retained_unchanged(data)
                self.loaded_images_match(data)
                self.atomic(
                    self.envfile, (self.record.parent / "candidate.env").read_bytes()
                )
                self.switch("candidate.compose.json")
                self.health(data, True)
                self.materials_matches(data, True)
                self.fresh_source(data)
                self.validate(data)
                require(
                    engine.digest(self.envfile) == data["candidate_environment_sha256"],
                    "candidate environment changed before commit",
                )
                require(
                    self.guard.full_baseline()
                    == (Path(data["baseline_runtime"]), data["baseline"]),
                    "full baseline changed before commit",
                )
                require(
                    self.marker.is_symlink()
                    and str(self.marker.resolve()) == data["marker_before"],
                    "scoped checkpoint changed before commit",
                )
                self.checkpoint(data, "committing")
                self.publish_full_alias(data, self.runtime)
                self.atomic(
                    self.watch / "last-activated-sha", (self.o.sha + "\n").encode()
                )
                require(
                    self.marker.is_symlink()
                    and str(self.marker.resolve()) == data["marker_before"],
                    "scoped marker changed before transition commit",
                )
                audit = self.record.parent / "scoped-marker.before"
                self.alias(audit, data["marker_before"])
                self.marker.unlink()
                self.durable_directory(self.watch)
                data["accepted_selected"] = self.snapshot(self.containers(), True)
                # From this durable boundary, automatic publications may use
                # the accepted stack. Reopening the original trigger must never
                # turn legitimate pending work into a container/data rollback.
                self.checkpoint(data, "accepted")
                self.restore_control_plane(data["materials"]["control_plane"])
                self.final_state(data, True)
                self.checkpoint(data, "active")
                return "active"
            except BaseException:
                if data["status"] not in ("accepted", "active"):
                    self.rollback(data)
                raise


if __name__ == "__main__":
    try:
        os.umask(0o077)
        release = Release(SELECTED)
        if SELECTED.preflight:
            release.preflight()
            result = "preflight_passed"
        else:
            result = release.execute()
        print(json.dumps({"status": result}))
    except Exception as error:
        message = (
            str(error)
            if isinstance(error, (engine.ReleaseError, OSError))
            or error.__class__.__name__ == "ReleaseError"
            else "verification failed (" + error.__class__.__name__ + ")"
        )
        print("stack transition: " + message, file=sys.stderr)
        raise SystemExit(1)
