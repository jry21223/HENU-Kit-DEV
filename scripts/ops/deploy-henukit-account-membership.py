#!/usr/bin/env python3
"""Release only Account Portfolio and one exact Bot route over the active Portal cohort."""

import argparse
import copy
import importlib.util
import hashlib
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

ROOT = Path(__file__).resolve().parent
BOUNDARY = Path("/")
if "--test-root" in sys.argv:
    BOUNDARY = Path(sys.argv[sys.argv.index("--test-root") + 1]).resolve()
    if (
        os.geteuid() == 0
        or BOUNDARY == Path(tempfile.gettempdir()).resolve()
        or not BOUNDARY.is_relative_to(Path(tempfile.gettempdir()).resolve())
    ):
        raise SystemExit("invalid private test boundary")


def load(name):
    path = ROOT / name
    for entry in (path, *path.parents):
        if entry == BOUNDARY:
            break
        info = entry.lstat()
        if entry.is_symlink() or info.st_uid != os.geteuid() or info.st_mode & 0o022:
            raise SystemExit("untrusted membership release dependency")
    spec = importlib.util.spec_from_file_location(name.replace(".", "_"), path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


engine = load("deploy-henukit-food-scoped.py")
portal = load("deploy-henukit-portal-successor.py")
require = engine.require
SERVICES = ("account-portfolio", "nginx")
BOT_KEYS = tuple(
    "ACCOUNT_PORTFOLIO_BOT_" + key for key in ("CLIENT_ID", "KEY_ID", "SECRET")
)
ROUTE = "/api/v1/bot/membership-entitlement"


class Release(engine.Release):
    services = SERVICES
    helper_file = Path(__file__)
    state_scope = "account-membership"
    receipt_format = "henukit-account-membership-v1"
    predecessor_rollbacks = ()

    def provenance(self):
        # Nginx keeps its retained official image; only the owner image is loaded.
        self.services = ("account-portfolio",)
        try:
            images = super().provenance()
        finally:
            self.services = SERVICES
        owner = self.archive(
            "henukit-account-portfolio-" + self.o.sha + ".docker.tar.gz"
        )
        proof = self.proof / (
            "henukit-membership-image-" + self.o.sha + ".attestation.json"
        )
        self.trusted(proof)
        self.run(
            "gh",
            "attestation",
            "verify",
            owner,
            "--repo",
            engine.REPO,
            "--bundle",
            proof,
            "--signer-workflow",
            engine.REPO + "/.github/workflows/deploy-henukit.yml",
            "--source-ref",
            "refs/heads/main",
            "--source-digest",
            self.o.sha,
            "--predicate-type",
            "https://github.com/jry21223/HENU-Kit-DEV/attestations/account-membership-image-v1",
            "--deny-self-hosted-runners",
            "--custom-trusted-root",
            self.proof / "trusted_root.jsonl",
            "--format",
            "json",
        )
        archive = self.archive("henukit-runtime-" + self.o.sha + ".tar.gz")
        with self.safe_tar(archive) as bundle:
            critical = [
                "bin/deploy-henukit-portal-successor.py",
                "infra/nginx/henukit.conf.example",
            ]
            critical.extend(
                "migrations/account-portfolio/" + p.name
                for p in (self.runtime / "migrations/account-portfolio").iterdir()
            )
            packaged = {
                m.name.removeprefix("./") for m in bundle.getmembers() if m.isfile()
            }
            require(
                {p for p in packaged if p.startswith("migrations/account-portfolio/")}
                == {
                    p for p in critical if p.startswith("migrations/account-portfolio/")
                },
                "owner migration inventory differs",
            )
            for name in critical:
                self.trusted(self.runtime / name)
                raw = bundle.extractfile("./" + name).read()
                require(
                    hashlib.sha256(raw).hexdigest()
                    == engine.digest(self.runtime / name),
                    "membership runtime differs from attestation",
                )
        return images

    def database_target(self, containers):
        from urllib.parse import parse_qsl, unquote, urlsplit

        owner = self.container_environment(containers["henukit-account-portfolio-1"])
        postgres = self.container_environment(containers["henukit-postgres-1"])
        target = urlsplit(owner.get("ACCOUNT_PORTFOLIO_DATABASE_URL", ""))
        require(
            target.scheme in ("postgres", "postgresql")
            and target.hostname == "postgres"
            and target.port in (None, 5432)
            and unquote(target.path) == "/account_portfolio"
            and unquote(target.username or "") == postgres.get("POSTGRES_USER")
            and unquote(target.password or "") == postgres.get("POSTGRES_PASSWORD")
            and not target.fragment
            and parse_qsl(target.query, keep_blank_values=True)
            in ([], [("sslmode", "disable")]),
            "owner backup target differs from current local database",
        )

    @staticmethod
    def account_projection(rendered):
        projection = {
            key: copy.deepcopy(rendered.get(key))
            for key in ("name", "networks", "volumes", "configs", "secrets")
        }
        account = copy.deepcopy(rendered["services"]["account-portfolio"])
        account.pop("image", None)
        environment = account.get("environment", {})
        for key in BOT_KEYS:
            environment.pop(key, None)
        projection["account"] = account
        return projection

    def route_configuration(self, containers):
        mounts = [
            p
            for p in containers["henukit-nginx-1"]["Mounts"]
            if p["Destination"] == "/etc/nginx/conf.d/default.conf"
        ]
        require(
            len(mounts) == 1 and mounts[0]["Type"] == "bind" and not mounts[0]["RW"],
            "unexpected Nginx mount",
        )
        source = Path(mounts[0]["Source"])
        self.trusted(source)
        before = source.read_bytes()
        require(
            ROUTE.encode() not in before,
            "Bot route already exists outside this receipt",
        )
        template = (self.runtime / "infra/nginx/henukit.conf.example").read_text()
        matches = re.findall(
            r"  location = /api/v1/bot/membership-entitlement \{[\s\S]*?\n  \}\n",
            template,
        )
        require(len(matches) == 1, "exact Bot route is absent or duplicated")
        anchor = b"  # EasyPay callback"
        require(before.count(anchor) == 1, "retained Nginx insertion boundary changed")
        return (
            source,
            before,
            before.replace(anchor, matches[0].encode() + b"\n" + anchor, 1),
        )

    def preflight(self):
        attempt, images = self.source(), self.provenance()
        guard = portal.Release(self.o)
        baseline, full_sha = guard.full_baseline()
        guard.pending()
        require(
            not self.record.parent.exists(), "prior attempt needs explicit rollback"
        )
        self.trusted(self.envfile, private=True)
        containers = self.containers()
        predecessor, previous = guard.predecessor(containers)
        for name, item in containers.items():
            if name in {"henukit-" + s + "-1" for s in engine.DEFERRED}:
                require(
                    not item["State"]["Running"],
                    "deferred services must remain stopped",
                )
            else:
                require(
                    item["State"]["Running"]
                    and item["State"].get("Health", {}).get("Status", "healthy")
                    == "healthy",
                    "unhealthy retained service",
                )
        require(
            containers["henukit-account-portfolio-1"]["Config"]["Image"]
            == "henukit-account-portfolio:" + full_sha,
            "owner is not on retained full baseline",
        )
        summary = self.container_environment(containers["henukit-portal-summary-1"])
        version, deployed = summary["PORTAL_VERSION"], summary["PORTAL_DEPLOYED_AT"]
        before = json.loads(
            self.compose(
                baseline, full_sha, version, deployed, "config", "--format", "json"
            )
        )
        after = json.loads(
            self.compose(
                self.runtime,
                self.o.sha,
                version,
                deployed,
                "config",
                "--format",
                "json",
            )
        )
        require(
            self.account_projection(before) == self.account_projection(after),
            "unrelated owner or shared configuration change",
        )
        keys = after["services"]["account-portfolio"].get("environment", {})
        require(
            all(isinstance(keys.get(key), str) and keys[key] for key in BOT_KEYS)
            and len(keys[BOT_KEYS[2]]) >= 32,
            "independent Bot credential missing",
        )
        require(
            keys[BOT_KEYS[0]]
            not in (
                keys.get("ACCOUNT_PORTFOLIO_SERVICE_CLIENT_ID"),
                keys.get("ACCOUNT_PORTFOLIO_CONSOLE_CLIENT_ID"),
            )
            and keys[BOT_KEYS[2]]
            not in (
                keys.get("ACCOUNT_PORTFOLIO_SERVICE_SECRET"),
                keys.get("ACCOUNT_PORTFOLIO_CONSOLE_SECRET"),
            ),
            "Bot credential reuses another authority",
        )
        self.running_configuration(
            containers, baseline, full_sha, version, deployed, before
        )
        self.rollback_images(self.snapshot(containers, True))
        for folder in ("migrations/account-portfolio",):

            def inventory(root):
                return {
                    p.name: engine.digest(p)
                    for p in (root / folder).iterdir()
                    if p.is_file()
                }

            require(
                inventory(baseline) == inventory(self.runtime),
                "membership scope prohibits migrations",
            )
        source, nginx_before, nginx_after = self.route_configuration(containers)
        require(
            shutil.disk_usage(self.artifacts).free >= 4096 * 1024 * 1024,
            "less than 4096 MiB free",
        )
        return dict(
            format=self.receipt_format,
            scope=list(SERVICES),
            sha=self.o.sha,
            run_id=self.o.run_id,
            attempt=attempt,
            baseline=full_sha,
            baseline_runtime=str(baseline),
            runtime=str(self.runtime),
            predecessor_record=str(predecessor),
            predecessor_receipt_sha256=engine.digest(predecessor),
            baseline_compose_sha256=engine.digest(
                baseline / "docker-compose.henukit.release.yml"
            ),
            candidate_compose_sha256=engine.digest(
                self.runtime / "docker-compose.henukit.release.yml"
            ),
            helper_sha256=engine.digest(self.helper_file),
            engine_sha256=engine.digest(ROOT / "deploy-henukit-food-scoped.py"),
            portal_helper_sha256=engine.digest(
                ROOT / "deploy-henukit-portal-successor.py"
            ),
            environment_sha256=engine.digest(self.envfile),
            selected=self.snapshot(containers, True),
            unselected=self.snapshot(containers),
            image_candidates=images,
            images={},
            old_version=version,
            old_deployed_at=deployed,
            nginx_source=str(source),
            nginx_before=nginx_before.decode(),
            nginx_after=nginx_after.decode(),
            before=before,
            after=after,
            getwork_acceptance="deferred",
            whole_application_release=False,
        )

    def validate(self, data):
        require(
            data["format"] == self.receipt_format
            and data["sha"] == self.o.sha
            and data["run_id"] == self.o.run_id
            and data["scope"] == list(SERVICES),
            "receipt identity mismatch",
        )
        for path, checksum in [
            (self.helper_file, data["helper_sha256"]),
            (ROOT / "deploy-henukit-food-scoped.py", data["engine_sha256"]),
            (ROOT / "deploy-henukit-portal-successor.py", data["portal_helper_sha256"]),
            (Path(data["predecessor_record"]), data["predecessor_receipt_sha256"]),
            (
                self.runtime / "docker-compose.henukit.release.yml",
                data["candidate_compose_sha256"],
            ),
            (
                Path(data["baseline_runtime"]) / "docker-compose.henukit.release.yml",
                data["baseline_compose_sha256"],
            ),
        ]:
            self.trusted(path)
            require(
                engine.digest(path) == checksum, "retained release identity changed"
            )
        guard = portal.Release(self.o)
        guard.predecessor_chain(Path(data["predecessor_record"]))
        baseline, sha = guard.full_baseline()
        require(
            str(baseline) == data["baseline_runtime"] and sha == data["baseline"],
            "full baseline changed",
        )
        require(
            self.marker.is_symlink()
            and self.marker.lstat().st_uid == self.uid
            and self.marker.resolve()
            in (Path(data["predecessor_record"]).parent, self.record.parent),
            "foreign scoped marker",
        )
        self.trusted(self.envfile, private=True)
        self.trusted(Path(data["nginx_source"]))
        if data.get("configuration_hashes"):
            for name, checksum in data["configuration_hashes"].items():
                path = self.record.parent / name
                self.trusted(path, private=True)
                require(
                    engine.digest(path) == checksum,
                    "private cohort configuration changed",
                )
        elif data.get("status") in (
            "prepared",
            "applying",
            "active",
            "rollback_failed",
            "rolling_back",
        ):
            raise engine.ReleaseError("private configuration proof missing")
        require(
            engine.digest(self.envfile) == data["environment_sha256"],
            "private environment changed",
        )
        require(
            self.snapshot(self.containers()) == data["unselected"],
            "unselected service changed",
        )
        require(
            Path(data["nginx_source"]).read_text() == data["nginx_before"],
            "retained Nginx file changed",
        )
        guard.pending()

    def configurations(self, data):
        for name, rendered in [("before", data["before"]), ("after", data["after"])]:
            config = {
                k: copy.deepcopy(rendered[k])
                for k in ("name", "networks", "volumes", "configs", "secrets")
                if k in rendered
            }
            config["services"] = {
                s: copy.deepcopy(rendered["services"][s]) for s in SERVICES
            }
            if name == "after":
                config["services"]["nginx"] = copy.deepcopy(
                    data["before"]["services"]["nginx"]
                )
                for mount in config["services"]["nginx"].get("volumes", []):
                    if mount.get("target") == "/etc/nginx/conf.d/default.conf":
                        mount["source"] = str(self.record.parent / "nginx.conf")
            for service in config["services"].values():
                service.pop("depends_on", None)
                service.pop("build", None)
            path = self.record.parent / (name + ".compose.json")
            self.private_write(path, json.dumps(self.literal_config(config)))
        self.private_write(self.record.parent / "nginx.conf", data["nginx_after"])
        data["configuration_hashes"] = {
            name: engine.digest(self.record.parent / name)
            for name in ("before.compose.json", "after.compose.json", "nginx.conf")
        }

    @classmethod
    def literal_config(cls, value):
        if isinstance(value, str):
            return value.replace("$", "$$")
        if isinstance(value, list):
            return [cls.literal_config(item) for item in value]
        if isinstance(value, dict):
            return {key: cls.literal_config(item) for key, item in value.items()}
        return value

    @staticmethod
    def private_write(path, value):
        descriptor = os.open(
            path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600
        )
        with os.fdopen(descriptor, "w") as stream:
            stream.write(value)
            stream.flush()
            os.fsync(stream.fileno())

    def backup(self, data):
        # Reuse the proven pg_dump/isolated-restore sequence with fixed owner names.
        # All database commands remain inside the existing PostgreSQL container.
        shutil.copy2(self.envfile, self.record.parent / "configuration.backup")
        require(
            engine.digest(self.record.parent / "configuration.backup")
            == data["environment_sha256"],
            "environment backup mismatch",
        )
        archive = self.record.parent / "account-portfolio.dump"
        dump = self.run(
            "docker",
            "exec",
            "henukit-postgres-1",
            "sh",
            "-ceu",
            'pg_dump -U "$POSTGRES_USER" -d account_portfolio -Fc',
            timeout=180,
        )
        require(dump, "empty owner backup")
        descriptor = os.open(archive, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(dump)
            stream.flush()
            os.fsync(stream.fileno())
        database = (
            "henukit_member_verify_" + self.o.sha[:8] + "_" + secrets.token_hex(6)
        )
        self.run(
            "docker",
            "exec",
            "henukit-postgres-1",
            "sh",
            "-ceu",
            'createdb -U "$POSTGRES_USER" -T template0 "$1"',
            "sh",
            database,
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
                database,
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
                    'psql -U "$POSTGRES_USER" -d "$1" -Atqc "SELECT count(*) FROM account_portfolio_memberships"',
                    "sh",
                    database,
                )
                .decode()
                .strip()
            )
            require(count.isdigit(), "restored owner schema unreadable")
        finally:
            self.run(
                "docker",
                "exec",
                "henukit-postgres-1",
                "sh",
                "-ceu",
                'dropdb -U "$POSTGRES_USER" "$1"',
                "sh",
                database,
            )
        data["backup_sha256"] = engine.digest(archive)
        data["restored_memberships"] = int(count)

    def switch(self, phase):
        self.run(
            "docker",
            "compose",
            "-f",
            self.record.parent / (phase + ".compose.json"),
            "up",
            "-d",
            "--no-deps",
            *SERVICES,
            timeout=180,
        )

    def healthy(self, data, candidate):
        expected = (
            data["images"]
            if candidate
            else {s: data["selected"]["henukit-" + s + "-1"]["image"] for s in SERVICES}
        )
        for _ in range(30):
            containers = self.containers()
            if all(
                containers["henukit-" + s + "-1"]["State"]["Running"]
                and containers["henukit-" + s + "-1"]["Image"] == expected[s]
                and containers["henukit-" + s + "-1"]["State"]
                .get("Health", {})
                .get("Status", "healthy")
                == "healthy"
                for s in SERVICES
            ):
                break
            if not self.testing:
                time.sleep(2)
        else:
            raise engine.ReleaseError("membership services did not become ready")
        self.validate(data)
        self.selected_configuration(data, candidate)
        self.run("docker", "exec", "henukit-nginx-1", "nginx", "-t")
        for path in ("/", "/food", "/library", "/practice", "/api/v1/healthz"):
            self.http(path, (200,))
        self.http(ROUTE, (401,) if candidate else (404,))

    def selected_configuration(self, data, candidate):
        path = self.record.parent / (
            "after.compose.json" if candidate else "before.compose.json"
        )
        self.trusted(path, private=True)
        rendered = json.loads(
            self.run("docker", "compose", "-f", path, "config", "--format", "json")
        )
        containers = self.containers()
        for service in SERVICES:
            item = containers["henukit-" + service + "-1"]
            expected = rendered["services"][service]
            actual = self.container_environment(item)
            require(
                all(
                    actual.get(key) == str(value)
                    for key, value in expected.get("environment", {}).items()
                ),
                "running selected environment differs",
            )
            require(
                item["Config"]["Image"] == expected["image"],
                "running selected image tag differs",
            )
            hashes = (
                self.run("docker", "compose", "-f", path, "config", "--hash", service)
                .decode()
                .split()
            )
            require(
                len(hashes) == 2
                and hashes[0] == service
                and item["Config"]
                .get("Labels", {})
                .get("com.docker.compose.config-hash")
                == hashes[1],
                "running selected ports/networks/configuration hash differs",
            )
        nginx = rendered["services"]["nginx"]
        mounts = [
            m
            for m in nginx.get("volumes", [])
            if m.get("target") == "/etc/nginx/conf.d/default.conf"
        ]
        actual = [
            m
            for m in containers["henukit-nginx-1"]["Mounts"]
            if m["Destination"] == "/etc/nginx/conf.d/default.conf"
        ]
        require(
            len(mounts) == len(actual) == 1
            and actual[0]["Type"] == "bind"
            and actual[0]["Source"] == mounts[0]["source"]
            and not actual[0]["RW"],
            "running Nginx mount differs",
        )

    def restore(self, data):
        self.validate(data)
        self.rollback_images(data["selected"])
        self.write_receipt(data, "rolling_back")
        self.switch("before")
        self.healthy(data, False)
        # Container identities change during restoration. Keep this receipt as
        # the checkpoint; the immutable Portal receipt remains its predecessor.
        data["selected_checkpoint"] = self.snapshot(self.containers(), True)
        self.write_receipt(data, "rolled_back")

    def checkpoint(self, data):
        require(
            data.get("selected_checkpoint") == self.snapshot(self.containers(), True),
            "membership checkpoint containers changed",
        )
        self.healthy(data, data["status"] == "active")

    def execute(self):
        if self.marker.is_symlink() and self.marker.resolve() == self.record.parent:
            self.trusted(self.record, private=True)
            data = json.loads(self.record.read_text())
            require(
                data["status"] == "active",
                "interrupted attempt requires explicit rollback",
            )
            self.checkpoint(data)
            return "already_active"
        data = self.preflight()
        with self.lock_watcher():
            self.validate(data)
            self.source()
            require(
                self.snapshot(self.containers(), True) == data["selected"],
                "selected baseline changed",
            )
            self.record.parent.mkdir(parents=True, mode=0o700)
            self.write_receipt(data, "preparing")
            self.backup(data)
            self.configurations(data)
            self.write_receipt(data, "prepared")
            self.run(
                "docker",
                "load",
                "--input",
                self.archive(
                    "henukit-account-portfolio-" + self.o.sha + ".docker.tar.gz"
                ),
                timeout=180,
            )
            image = json.loads(
                self.run(
                    "docker",
                    "image",
                    "inspect",
                    "henukit-account-portfolio:" + self.o.sha,
                )
            )[0]
            require(
                image["Id"] in data["image_candidates"]["account-portfolio"]
                and (image["Os"], image["Architecture"]) == ("linux", "amd64"),
                "loaded owner image identity mismatch",
            )
            data["images"] = {
                "account-portfolio": image["Id"],
                "nginx": data["selected"]["henukit-nginx-1"]["image"],
            }
            self.source()
            self.validate(data)
            require(
                shutil.disk_usage(self.artifacts).free >= 4096 * 1024 * 1024,
                "less than 4096 MiB before switch",
            )
            self.write_receipt(data, "applying")
            link = self.watch / (".membership-incoming-" + secrets.token_hex(8))
            link.symlink_to(self.record.parent)
            os.replace(link, self.marker)
            self.durable_directory(self.watch)
            try:
                self.switch("after")
                self.healthy(data, True)
                self.source()
                data["selected_checkpoint"] = self.snapshot(self.containers(), True)
                self.write_receipt(data, "active")
            except Exception:
                try:
                    self.restore(data)
                except Exception:
                    self.write_receipt(data, "rollback_failed")
                    raise
                raise
        return "active"

    def rollback(self):
        self.trusted(self.record, private=True)
        data = json.loads(self.record.read_text())
        with self.lock_watcher():
            if data["status"] == "rolled_back":
                if "selected_checkpoint" in data:
                    self.checkpoint(data)
                else:
                    self.validate(data)
                    require(
                        self.snapshot(self.containers(), True) == data["selected"],
                        "pre-switch baseline changed",
                    )
                return "already_rolled_back"
            if data["status"] in ("preparing", "prepared"):
                self.validate(data)
                require(
                    self.snapshot(self.containers(), True) == data["selected"],
                    "pre-switch baseline changed",
                )
                self.write_receipt(data, "rolled_back")
                return "rolled_back"
            require(
                data["status"]
                in ("active", "applying", "rollback_failed", "rolling_back"),
                "unsupported receipt status",
            )
            if data["status"] == "active":
                self.validate(data)
                current = self.snapshot(self.containers(), True)
                saved = data.get("selected_checkpoint", {})
                require(
                    set(current) == set(saved)
                    and all(
                        all(
                            current[name][key] == saved[name][key]
                            for key in ("id", "image", "tag")
                        )
                        for name in current
                    ),
                    "membership checkpoint ownership changed",
                )
                self.selected_configuration(data, True)
            self.restore(data)
        return "rolled_back"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("sha", "run-id", "proof-dir"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--test-root")
    parser.add_argument(
        "--acknowledge-getwork-deferred", action="store_true", required=True
    )
    modes = parser.add_mutually_exclusive_group(required=True)
    for name in ("preflight", "execute", "rollback"):
        modes.add_argument("--" + name, action="store_true")
    options = parser.parse_args()
    os.umask(0o077)
    require(
        re.fullmatch(r"[0-9a-f]{40}", options.sha) and options.run_id.isdigit(),
        "invalid release identity",
    )
    release = Release(options)
    if options.preflight:
        release.preflight()
        status = "preflight_passed"
    else:
        status = release.execute() if options.execute else release.rollback()
    print(
        json.dumps(
            {
                "status": status,
                "scope": list(SERVICES),
                "getwork_acceptance": "deferred",
            }
        )
    )


if __name__ == "__main__":
    try:
        main()
    except (
        engine.ReleaseError,
        portal.engine.ReleaseError,
        KeyError,
        OSError,
        ValueError,
        subprocess.TimeoutExpired,
        tarfile.TarError,
    ) as error:
        print("account-membership: " + str(error), file=sys.stderr)
        sys.exit(1)
