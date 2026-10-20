#!/usr/bin/env python3
"""Fixed three-unit release. Full release and getWork health remain separate facts."""

import argparse
import contextlib
import fcntl
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
from urllib.parse import parse_qsl, unquote, urlsplit

SERVICES = ("food", "portal-gateway", "portal")
DEFERRED = ("career-opportunities", "career-mcp", "getwork-mcp-relay")
REPO = "jry21223/HENU-Kit-DEV"
# The one attested predecessor whose first-hop legacy check could fail rollback.
PREDECESSOR_ROLLBACK = (
    "c1f4cddabd544bda7511084b499f9e4feb15e8a0",
    "ef5e9658045b2b34196fba4bf447b5cdca59772ce726a406eb3a9762937453d8",
)


class ReleaseError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise ReleaseError(message)


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def archive_image_ids(archive, config):
    config_id = "sha256:" + hashlib.sha256(config).hexdigest()
    identities = [config_id]
    if "index.json" not in archive.getnames():
        return identities
    index = json.load(archive.extractfile("index.json"))
    manifests = index.get("manifests", [])
    require(index.get("schemaVersion") == 2 and len(manifests) == 1, "OCI archive must bind one manifest")
    descriptor = manifests[0]
    media_types = (
        "application/vnd.oci.image.manifest.v1+json",
        "application/vnd.docker.distribution.manifest.v2+json",
    )
    manifest_id = descriptor.get("digest", "")
    require(
        descriptor.get("mediaType") in media_types and re.fullmatch(r"sha256:[0-9a-f]{64}", manifest_id),
        "invalid OCI image descriptor",
    )
    raw = archive.extractfile("blobs/sha256/" + manifest_id.removeprefix("sha256:")).read()
    require(
        hashlib.sha256(raw).hexdigest() == manifest_id.removeprefix("sha256:")
        and descriptor.get("size") == len(raw),
        "OCI manifest digest/size mismatch",
    )
    manifest = json.loads(raw)
    binding = manifest.get("config", {})
    require(
        manifest.get("schemaVersion") == 2
        and manifest.get("mediaType") == descriptor["mediaType"]
        and binding.get("digest") == config_id
        and binding.get("size") == len(config),
        "OCI manifest config binding mismatch",
    )
    identities.append(manifest_id)
    return identities


class Release:
    def __init__(self, options):
        self.o = options
        self.prefix = Path(options.test_root).resolve() if options.test_root else Path("/")
        self.testing = bool(options.test_root)
        if self.testing:
            require(os.geteuid() != 0, "test mode is forbidden as root")
            require(
                self.prefix != Path(tempfile.gettempdir()).resolve()
                and self.prefix.is_relative_to(Path(tempfile.gettempdir()).resolve()),
                "test root must be a private temporary directory",
            )
        else:
            require(os.geteuid() == 0, "release requires root")
            require(
                os.uname().sysname == "Linux" and os.uname().machine == "x86_64",
                "production host must be Linux amd64",
            )
        self.uid = os.geteuid()
        self.runtime = Path(__file__).resolve().parent.parent
        self.envfile = self.path("/opt/henukit/.env.henukit")
        self.watch = self.path("/var/lib/henukit-actions-watch")
        self.state = self.path("/var/lib/henukit-scoped-releases/food-author-edit")
        self.record = self.state / options.sha / "receipt.json"
        self.marker = self.watch / "scoped-food-release"
        self.artifacts = self.path("/opt/henukit-staging") / options.sha
        self.proof = Path(options.proof_dir)
        self.tools = {}
        for name in ("docker", "gh", "systemctl", "curl"):
            tool = Path(shutil.which(name) or "/missing") if self.testing else Path("/usr/bin") / name
            self.trusted(tool)
            if self.testing:
                require(tool.parent == self.prefix / "bin", "test commands must be fixture executables")
            self.tools[name] = str(tool)
        self.env = dict(os.environ)
        self.env["GH_HOST"] = "github.com"
        if not options.rollback:
            token = self.path("/etc/henukit/github-actions-read.token")
            self.trusted(token, private=True)
            self.env["GH_TOKEN"] = token.read_text().strip()
            require(self.env["GH_TOKEN"], "existing Actions read token is empty")

    def path(self, value):
        return self.prefix / value.lstrip("/")

    def trusted(self, path, private=False):
        require(path.is_absolute(), "trusted path must be absolute")
        require(not path.is_symlink() and path.is_file(), "trusted file missing or symlinked: " + path.name)
        info = path.stat()
        require(
            info.st_uid == self.uid and not info.st_mode & 0o022,
            "untrusted file ownership/mode: " + path.name,
        )
        if private:
            require(info.st_mode & 0o777 in (0o400, 0o600), "private file mode must be 0400/0600")
        parent = path.parent
        while parent != self.prefix:
            info = parent.lstat()
            require(
                parent.is_dir()
                and not parent.is_symlink()
                and info.st_uid == self.uid
                and not info.st_mode & 0o022,
                "untrusted parent: " + str(parent),
            )
            require(parent != parent.parent, "trusted path is outside test boundary")
            parent = parent.parent
        if self.testing:
            require(path.is_relative_to(self.prefix), "test file outside fixture")

    def trusted_directory(self, path):
        while path != self.prefix:
            info = path.lstat()
            require(
                path.is_dir()
                and not path.is_symlink()
                and info.st_uid == self.uid
                and not info.st_mode & 0o022,
                "untrusted state directory",
            )
            require(path != path.parent, "directory outside trust boundary")
            path = path.parent

    @staticmethod
    def durable_directory(path):
        descriptor = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)

    def run(self, name, *args, env=None, input=None, timeout=60):
        result = subprocess.run(
            [self.tools[name], *map(str, args)],
            env=env or self.env,
            input=input,
            capture_output=True,
            timeout=timeout,
        )
        require(result.returncode == 0, name + " command failed (" + str(result.returncode) + ")")
        return result.stdout

    def source(self):
        head = self.run("gh", "api", "repos/" + REPO + "/git/ref/heads/main").decode().strip()
        # gh --jq is explicit so the value is not inferred from a local checkout.
        if head.startswith("{"):
            head = json.loads(head)["object"]["sha"]
        require(head == self.o.sha, "candidate is not current main")
        run = json.loads(
            self.run(
                "gh",
                "run",
                "view",
                self.o.run_id,
                "--repo",
                REPO,
                "--json",
                "headSha,headBranch,event,status,conclusion,workflowName,attempt",
            )
        )
        require(
            (
                run["headSha"],
                run["headBranch"],
                run["event"],
                run["status"],
                run["conclusion"],
                run["workflowName"],
            )
            == (self.o.sha, "main", "push", "completed", "success", "Build HENU Kit release artifacts"),
            "source workflow is not successful exact-main push",
        )
        newest = json.loads(
            self.run(
                "gh",
                "run",
                "list",
                "--repo",
                REPO,
                "--workflow",
                "deploy-henukit.yml",
                "--branch",
                "main",
                "--event",
                "push",
                "--limit",
                "1",
                "--json",
                "databaseId,attempt",
            )
        )
        require(
            len(newest) == 1
            and str(newest[0]["databaseId"]) == self.o.run_id
            and newest[0]["attempt"] == run["attempt"],
            "not the newest workflow attempt",
        )
        return run["attempt"]

    def archive(self, name):
        path = self.artifacts / name
        self.trusted(path)
        self.trusted(Path(str(path) + ".sha256"))
        require(
            Path(str(path) + ".sha256").read_text().strip().split() == [digest(path), name],
            "archive checksum mismatch: " + name,
        )
        return path

    def safe_tar(self, path):
        archive = tarfile.open(path, "r:gz")
        names = set()
        for member in archive.getmembers():
            name = member.name.removeprefix("./")
            require(
                not name.startswith("/")
                and ".." not in Path(name).parts
                and (member.isfile() or member.isdir())
                and name not in names,
                "unsafe archive entry",
            )
            names.add(name)
        return archive

    def provenance(self):
        self.trusted(Path(__file__).absolute())
        self.trusted(self.runtime / "RELEASE_SHA")
        require((self.runtime / "RELEASE_SHA").read_text().strip() == self.o.sha, "runtime SHA mismatch")
        manifest = self.proof / ("henukit-getwork-actions-" + self.o.sha + ".manifest")
        bundle = self.proof / ("henukit-getwork-actions-" + self.o.sha + ".attestation.json")
        trusted_root = self.proof / "trusted_root.jsonl"
        for path in (manifest, bundle, trusted_root):
            self.trusted(path)
        self.run(
            "gh",
            "attestation",
            "verify",
            manifest,
            "--repo",
            REPO,
            "--bundle",
            bundle,
            "--signer-workflow",
            REPO + "/.github/workflows/deploy-henukit.yml",
            "--source-ref",
            "refs/heads/main",
            "--source-digest",
            self.o.sha,
            "--predicate-type",
            "https://github.com/jry21223/HENU-Kit-DEV/attestations/getwork-actions-release-v1",
            "--deny-self-hosted-runners",
            "--custom-trusted-root",
            trusted_root,
            "--format",
            "json",
        )
        runtime = self.archive("henukit-runtime-" + self.o.sha + ".tar.gz")
        require(
            "artifact_sha256=" + digest(runtime) + "  " + runtime.name in manifest.read_text().splitlines(),
            "attestation does not bind runtime",
        )
        with self.safe_tar(runtime) as archive:
            source = archive.extractfile("./bin/deploy-henukit-food-scoped.py").read()
            require(
                hashlib.sha256(source).hexdigest() == digest(Path(__file__)),
                "executed helper differs from attested runtime",
            )
            critical = {"RELEASE_SHA", "docker-compose.henukit.release.yml"}
            critical.update({"bin/watch-henukit-actions.sh", "bin/activate-henukit-release.sh"})
            critical.update("migrations/food/" + p.name for p in (self.runtime / "migrations/food").iterdir())
            packaged = {m.name.removeprefix("./") for m in archive.getmembers() if m.isfile()}
            require(
                {p for p in packaged if p.startswith("migrations/food/")}
                == {p for p in critical if p.startswith("migrations/food/")},
                "runtime Food migration inventory differs from attestation",
            )
            for name in critical:
                self.trusted(self.runtime / name)
                content = archive.extractfile("./" + name).read()
                require(
                    hashlib.sha256(content).hexdigest() == digest(self.runtime / name),
                    "runtime content differs from attestation: " + name,
                )
        for source, installed in (
            ("watch-henukit-actions.sh", "watch-henukit-actions"),
            ("activate-henukit-release.sh", "activate-henukit-release"),
        ):
            path = self.path("/usr/local/sbin") / installed
            self.trusted(path)
            require(
                os.access(path, os.X_OK) and digest(path) == digest(self.runtime / "bin" / source),
                "installed full-release guard differs from attested runtime",
            )
        images = {}
        for service in SERVICES:
            tag = "henukit-" + service + ":" + self.o.sha
            path = self.archive("henukit-" + service + "-" + self.o.sha + ".docker.tar.gz")
            with self.safe_tar(path) as archive:
                entries = json.load(archive.extractfile("manifest.json"))
                require(len(entries) == 1 and entries[0]["RepoTags"] == [tag], "image archive tag mismatch")
                config = archive.extractfile(entries[0]["Config"]).read()
                platform = json.loads(config)
                require(
                    (platform["os"], platform["architecture"]) == ("linux", "amd64"),
                    "image must be linux/amd64",
                )
                images[service] = archive_image_ids(archive, config)
        return images

    def containers(self):
        ids = self.run("docker", "ps", "-aq").decode().split()
        require(ids, "no current containers")
        return {item["Name"].lstrip("/"): item for item in json.loads(self.run("docker", "inspect", *ids))}

    @staticmethod
    def snapshot(containers, selected=False):
        return {
            name: {
                "id": item["Id"],
                "image": item["Image"],
                "tag": item["Config"]["Image"],
                "status": item["State"]["Status"],
                "running": item["State"]["Running"],
                "restarts": item["RestartCount"],
                "started_at": item["State"].get("StartedAt", ""),
                "finished_at": item["State"].get("FinishedAt", ""),
                "exit_code": item["State"].get("ExitCode", 0),
                "pid": item["State"].get("Pid", 0),
            }
            for name, item in containers.items()
            if (name in {"henukit-" + s + "-1" for s in SERVICES}) == selected
        }

    def compose(self, runtime, sha, version, deployed_at, *args):
        env = {**self.env, "RELEASE_SHA": sha, "PORTAL_VERSION": version, "PORTAL_DEPLOYED_AT": deployed_at}
        return self.run(
            "docker",
            "compose",
            "--env-file",
            self.envfile,
            "-f",
            runtime / "docker-compose.henukit.release.yml",
            *args,
            env=env,
            timeout=180,
        )

    @staticmethod
    def container_environment(item):
        return dict(value.split("=", 1) for value in item["Config"]["Env"] if "=" in value)

    def rollback_images(self, selected):
        for previous in selected.values():
            image = json.loads(self.run("docker", "image", "inspect", previous["tag"]))[0]
            require(
                (image["Id"], image["Os"], image["Architecture"]) == (previous["image"], "linux", "amd64"),
                "retained rollback image missing or changed",
            )

    def running_configuration(self, containers, baseline, old, version, deployed_at, rendered):
        for service in SERVICES:
            item = containers["henukit-" + service + "-1"]
            actual = self.container_environment(item)
            expected = rendered["services"][service].get("environment", {})
            require(
                all(actual.get(key) == str(value) for key, value in expected.items()),
                "running configuration environment differs from retained Compose: " + service,
            )
            hashes = (
                self.compose(baseline, old, version, deployed_at, "config", "--hash", service)
                .decode()
                .split()
            )
            require(
                len(hashes) == 2
                and hashes[0] == service
                and item["Config"].get("Labels", {}).get("com.docker.compose.config-hash") == hashes[1],
                "running configuration contract differs from retained Compose: " + service,
            )
        # This deliberately supports only the existing local Food database.
        # Never claim a successful backup of a configurable remote/alternate DB.
        food = self.container_environment(containers["henukit-food-1"])
        postgres = self.container_environment(containers["henukit-postgres-1"])
        target = urlsplit(food.get("FOOD_DATABASE_URL", ""))
        require(
            target.scheme in ("postgres", "postgresql")
            and target.hostname == "postgres"
            and target.port in (None, 5432)
            and unquote(target.path) == "/food"
            and unquote(target.username or "") == postgres.get("POSTGRES_USER")
            and unquote(target.password or "") == postgres.get("POSTGRES_PASSWORD")
            and not target.fragment
            and parse_qsl(target.query, keep_blank_values=True) in ([], [("sslmode", "disable")]),
            "Food backup target does not match the running local database",
        )

    def preflight(self):
        attempt = self.source()
        images = self.provenance()
        current = self.path("/opt/henukit/current")
        require(
            current.is_symlink() and current.lstat().st_uid == self.uid, "current must be a trusted symlink"
        )
        baseline = current.resolve()
        legacy = self.path("/opt/henukit-current")
        require(
            legacy.is_symlink() and legacy.lstat().st_uid == self.uid and legacy.resolve() == baseline,
            "full release aliases disagree",
        )
        for path in (baseline / "RELEASE_SHA", baseline / "docker-compose.henukit.release.yml", self.envfile):
            self.trusted(path)
        self.trusted(self.envfile, private=True)
        old = (baseline / "RELEASE_SHA").read_text().strip()
        require(re.fullmatch("[0-9a-f]{40}", old) and old != self.o.sha, "invalid or same baseline")
        self.trusted(self.watch / "last-activated-sha")
        for folder in (self.watch / "approvals", self.watch / "rollback-contracts/pending"):
            self.trusted_directory(folder)
        require(
            (self.watch / "last-activated-sha").read_text().strip() == old, "full baseline marker mismatch"
        )
        pending_approvals = []
        for entry in (self.watch / "approvals").iterdir():
            if entry.name in ("consumed", "revoked"):
                self.trusted_directory(entry)
            else:
                pending_approvals.append(entry)
        require(
            not pending_approvals and not any((self.watch / "rollback-contracts/pending").iterdir()),
            "pending full release must be reconciled first",
        )
        require(not self.marker.exists() and not self.marker.is_symlink(), "a scoped release already exists")
        require(not self.record.parent.exists(), "prior attempt requires explicit rollback or inspection")
        containers = self.containers()
        for service in SERVICES:
            item = containers["henukit-" + service + "-1"]
            require(
                item["State"]["Running"] and item["Config"]["Image"] == "henukit-" + service + ":" + old,
                "selected service is not on exact full baseline",
            )
        for service in DEFERRED:
            item = containers["henukit-" + service + "-1"]
            require(not item["State"]["Running"], "deferred dependency must remain stopped")
        for name, item in containers.items():
            if name not in {"henukit-" + s + "-1" for s in DEFERRED}:
                require(
                    item["State"]["Running"]
                    and item["State"].get("Health", {}).get("Status", "healthy") == "healthy",
                    "non-deferred baseline service unhealthy: " + name,
                )
        for runtime in (baseline, self.runtime):
            self.trusted(runtime / "docker-compose.henukit.release.yml")
        summary_env = self.container_environment(containers["henukit-portal-summary-1"])
        version = summary_env["PORTAL_VERSION"]
        deployed_at = summary_env["PORTAL_DEPLOYED_AT"]
        configs = []
        for runtime, sha in ((baseline, old), (self.runtime, self.o.sha)):
            data = json.loads(self.compose(runtime, sha, version, deployed_at, "config", "--format", "json"))
            config = {key: data.get(key) for key in ("name", "networks", "volumes", "configs", "secrets")}
            config["services"] = {s: dict(data["services"][s]) for s in SERVICES}
            for value in config["services"].values():
                value.pop("image", None)
            configs.append(config)
        require(configs[0] == configs[1], "selected service configuration is incompatible")
        self.running_configuration(containers, baseline, old, version, deployed_at, configs[0])
        self.rollback_images(self.snapshot(containers, True))

        def migrations(runtime):
            folder = runtime / "migrations/food"
            result = {}
            for path in folder.iterdir():
                self.trusted(path)
                result[path.name] = digest(path)
            return result

        require(
            migrations(baseline) == migrations(self.runtime),
            "Food schema changes require full migration release",
        )
        require(shutil.disk_usage(self.artifacts).free >= 4096 * 1024 * 1024, "less than 4096 MiB free")
        return {
            "format": "henukit-food-scoped-v1",
            "sha": self.o.sha,
            "run_id": self.o.run_id,
            "attempt": attempt,
            "baseline": old,
            "baseline_runtime": str(baseline),
            "runtime": str(self.runtime),
            "selected": self.snapshot(containers, True),
            "unselected": self.snapshot(containers),
            "image_candidates": images,
            "images": {},
            "environment_sha256": digest(self.envfile),
            "old_version": version,
            "baseline_compose_sha256": digest(baseline / "docker-compose.henukit.release.yml"),
            "helper_sha256": digest(Path(__file__)),
            "old_deployed_at": deployed_at,
            "getwork_acceptance": "deferred",
            "whole_application_release": False,
        }

    def write_receipt(self, data, status):
        data["status"] = status
        data["recorded_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        self.record.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        self.trusted_directory(self.record.parent)
        temporary = self.record.with_suffix(".incoming")
        with temporary.open("x") as stream:
            json.dump(data, stream, sort_keys=True)
            stream.flush()
            os.fsync(stream.fileno())
        temporary.chmod(0o600)
        os.replace(temporary, self.record)
        audit = self.record.parent / (status + "-" + secrets.token_hex(8) + ".json")
        with audit.open("x") as stream:
            json.dump(data, stream, sort_keys=True)
            stream.flush()
            os.fsync(stream.fileno())
        audit.chmod(0o400)
        self.durable_directory(self.record.parent)

    @contextlib.contextmanager
    def lock_watcher(self):
        # Use the same direct-deploy lock and safe-boundary protocol as the
        # signed transport. No forced service stop can interrupt an activation.
        self.trusted_directory(self.watch)
        descriptor = os.open(self.watch / "direct-deploy.lock", os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        with os.fdopen(descriptor, "a+") as direct:
            self.trusted(self.watch / "direct-deploy.lock")
            fcntl.flock(direct, fcntl.LOCK_EX | fcntl.LOCK_NB)
            active = (
                subprocess.run(
                    [self.tools["systemctl"], "is-active", "--quiet", "henukit-actions-watch.service"],
                    capture_output=True,
                ).returncode
                == 0
            )
            request = self.watch / "quiesce.request"
            acknowledged = self.watch / "quiesced"
            owned = False
            try:
                if request.exists() or request.is_symlink():
                    self.trusted(request, private=True)
                    tokens = request.read_text().split()
                    require(
                        len(tokens) == 3
                        and tokens[0] == self.o.sha
                        and tokens[1].isdigit()
                        and re.fullmatch("[0-9a-f]{32}", tokens[2]),
                        "quiesce request belongs to another attempt",
                    )
                    value = " ".join(tokens)
                    pid = tokens[1]
                    owned = True
                    active = True
                elif active:
                    instance = self.watch / "watcher.instance"
                    self.trusted(instance, private=True)
                    pid = instance.read_text().strip()
                    require(pid.isdigit() and pid != "0", "invalid watcher instance")
                    actual = (
                        self.run(
                            "systemctl",
                            "show",
                            "--property",
                            "MainPID",
                            "--value",
                            "henukit-actions-watch.service",
                        )
                        .decode()
                        .strip()
                    )
                    require(pid == actual, "watcher instance changed")
                    if acknowledged.exists():
                        self.trusted(acknowledged, private=True)
                        acknowledged.unlink()
                    value = self.o.sha + " " + pid + " " + secrets.token_hex(16)
                    temporary = self.watch / (".quiesce.request." + secrets.token_hex(8))
                    temporary.write_text(value + "\n")
                    temporary.chmod(0o600)
                    os.replace(temporary, request)
                    owned = True
                    self.durable_directory(self.watch)
                if owned:
                    for _ in range(120):
                        result = subprocess.run(
                            [
                                self.tools["systemctl"],
                                "is-active",
                                "--quiet",
                                "henukit-actions-watch.service",
                            ],
                            capture_output=True,
                        )
                        if result.returncode != 0:
                            break
                        require(
                            self.run(
                                "systemctl",
                                "show",
                                "--property",
                                "MainPID",
                                "--value",
                                "henukit-actions-watch.service",
                            )
                            .decode()
                            .strip()
                            == pid,
                            "watcher restarted before quiescence",
                        )
                        time.sleep(1)
                    else:
                        raise ReleaseError("watcher did not reach safe boundary")
                    self.trusted(acknowledged, private=True)
                    require(
                        acknowledged.read_text().strip() == value, "watcher quiescence was not acknowledged"
                    )
                descriptor = os.open(
                    self.watch / "watcher.lock", os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600
                )
                with os.fdopen(descriptor, "a+") as watcher:
                    self.trusted(self.watch / "watcher.lock", private=True)
                    fcntl.flock(watcher, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    yield
            finally:
                if owned:
                    request.unlink(missing_ok=True)
                    acknowledged.unlink(missing_ok=True)
                if active:
                    self.run("systemctl", "start", "henukit-actions-watch.service")
                    self.run("systemctl", "is-active", "--quiet", "henukit-actions-watch.service")

    def backup(self, data):
        folder = self.record.parent
        shutil.copy2(self.envfile, folder / "configuration.backup")
        (folder / "configuration.backup").chmod(0o600)
        require(
            digest(folder / "configuration.backup") == data["environment_sha256"],
            "configuration changed during backup",
        )
        archive = folder / "food.dump"
        with archive.open("xb") as output:
            result = subprocess.run(
                [
                    self.tools["docker"],
                    "exec",
                    "henukit-postgres-1",
                    "sh",
                    "-ceu",
                    'pg_dump -U "$POSTGRES_USER" -d food -Fc',
                ],
                env=self.env,
                stdout=output,
                stderr=subprocess.PIPE,
                timeout=180,
            )
            output.flush()
            os.fsync(output.fileno())
        archive.chmod(0o600)
        require(result.returncode == 0 and archive.stat().st_size > 0, "Food database backup failed")
        database = "henukit_food_verify_" + self.o.sha[:8] + "_" + secrets.token_hex(6)
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
            with archive.open("rb") as stream:
                result = subprocess.run(
                    [
                        self.tools["docker"],
                        "exec",
                        "-i",
                        "henukit-postgres-1",
                        "sh",
                        "-ceu",
                        'pg_restore --exit-on-error --no-owner --no-privileges -U "$POSTGRES_USER" -d "$1"',
                        "sh",
                        database,
                    ],
                    env=self.env,
                    stdin=stream,
                    capture_output=True,
                    timeout=180,
                )
            require(result.returncode == 0, "isolated Food database restore failed")
            count = (
                self.run(
                    "docker",
                    "exec",
                    "henukit-postgres-1",
                    "sh",
                    "-ceu",
                    'psql -U "$POSTGRES_USER" -d "$1" -Atqc "SELECT count(*) FROM food_posts"',
                    "sh",
                    database,
                )
                .decode()
                .strip()
            )
            require(count.isdigit(), "restored Food schema could not be read")
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
        data["backup_sha256"] = digest(archive)
        data["backup_bytes"] = archive.stat().st_size
        data["restored_food_posts"] = int(count)

    def unchanged(self, data):
        require(digest(self.envfile) == data["environment_sha256"], "production environment changed")
        require(
            self.path("/opt/henukit/current").resolve() == Path(data["baseline_runtime"])
            and self.path("/opt/henukit-current").resolve() == Path(data["baseline_runtime"]),
            "full release alias changed",
        )
        require(
            (self.watch / "last-activated-sha").read_text().strip() == data["baseline"],
            "full release marker changed",
        )
        require(
            self.snapshot(self.containers()) == data["unselected"],
            "unselected container identity/state changed",
        )

    def http(self, path, expected, method="GET"):
        for _ in range(30):
            try:
                with tempfile.NamedTemporaryFile() as output:
                    result = self.run(
                        "curl",
                        "--silent",
                        "--show-error",
                        "--max-time",
                        "5",
                        "--output",
                        output.name,
                        "--write-out",
                        "%{http_code} %{redirect_url}",
                        "--request",
                        method,
                        "https://henukit.cn" + path,
                    )
                    status, _, redirect = result.decode().strip().partition(" ")
                    if int(status) in expected:
                        return
                    if (
                        method == "GET"
                        and path == "/quiz/"
                        and status == "308"
                        and redirect == "https://henukit.cn/quiz"
                    ):
                        path = "/quiz"
            except (ReleaseError, subprocess.TimeoutExpired):
                pass
            if not self.testing:
                time.sleep(2)
        raise ReleaseError("public acceptance failed: " + path)

    def healthy(self, data, candidate):
        expected_sha = data["sha"] if candidate else data["baseline"]
        for _ in range(30):
            containers = self.containers()
            ready = True
            for service in SERVICES:
                item = containers["henukit-" + service + "-1"]
                image = (
                    data["images"][service]
                    if candidate
                    else data["selected"]["henukit-" + service + "-1"]["image"]
                )
                ready &= (
                    item["State"]["Running"]
                    and item["Config"]["Image"] == "henukit-" + service + ":" + expected_sha
                    and item["Image"] == image
                    and item["State"].get("Health", {}).get("Status", "healthy") == "healthy"
                )
            if ready:
                break
            if not self.testing:
                time.sleep(2)
        else:
            raise ReleaseError("selected services did not become ready")
        self.unchanged(data)
        for path in ("/", "/food", "/library", "/practice", "/api/v1/healthz", "/api/v1/library/materials"):
            self.http(path, (200,))
        self.http("/api/v1/account/summary", (200, 401, 403))
        self.http("/quiz/", (404,))
        self.http("/study-api/healthz", (404,))
        if candidate:
            probe = "/api/v1/food/posts/11111111-1111-4111-8111-111111111111"
            self.http(probe + "/edit", (401,))
            self.http(probe, (401,), "PUT")

    def restore(self, data):
        require(digest(self.envfile) == data["environment_sha256"], "cannot roll back changed environment")
        require(
            self.path("/opt/henukit/current").resolve() == Path(data["baseline_runtime"])
            and self.path("/opt/henukit-current").resolve() == Path(data["baseline_runtime"])
            and (self.watch / "last-activated-sha").read_text().strip() == data["baseline"],
            "cannot roll back a different full release",
        )
        compose = Path(data["baseline_runtime"]) / "docker-compose.henukit.release.yml"
        self.trusted(compose)
        require(digest(compose) == data["baseline_compose_sha256"], "retained Compose contract changed")
        self.rollback_images(data["selected"])
        self.compose(
            Path(data["baseline_runtime"]),
            data["baseline"],
            data["old_version"],
            data["old_deployed_at"],
            "up",
            "-d",
            "--no-deps",
            *SERVICES,
        )
        self.healthy(data, False)
        self.write_receipt(data, "rolled_back")
        self.remove_owned_marker()

    def remove_owned_marker(self):
        if self.marker.is_symlink() and self.marker.resolve() == self.record.parent:
            self.marker.unlink()
            self.durable_directory(self.watch)

    def execute(self):
        if self.marker.is_symlink() and self.marker.resolve() == self.record.parent:
            self.trusted(self.record, private=True)
            data = json.loads(self.record.read_text())
            require(
                data["sha"] == self.o.sha and data["helper_sha256"] == digest(Path(__file__)),
                "active receipt identity mismatch",
            )
            require(data["status"] == "active", "interrupted attempt requires explicit rollback")
            self.healthy(data, True)
            return "already_active"
        data = self.preflight()
        with self.lock_watcher():
            self.unchanged(data)
            require(self.snapshot(self.containers(), True) == data["selected"], "selected baseline changed")
            self.source()
            self.record.parent.mkdir(parents=True, mode=0o700)
            self.write_receipt(data, "preparing")
            self.backup(data)
            self.write_receipt(data, "prepared")
            for service in SERVICES:
                archive = self.archive("henukit-" + service + "-" + self.o.sha + ".docker.tar.gz")
                self.run("docker", "load", "--input", archive, timeout=180)
                image = json.loads(
                    self.run("docker", "image", "inspect", "henukit-" + service + ":" + self.o.sha)
                )[0]
                require(
                    image["Id"] in data["image_candidates"][service]
                    and (image["Os"], image["Architecture"]) == ("linux", "amd64"),
                    "loaded image identity/platform differs from archive",
                )
                data["images"][service] = image["Id"]
            self.source()
            self.unchanged(data)
            require(
                shutil.disk_usage(self.artifacts).free >= 4096 * 1024 * 1024,
                "less than 4096 MiB free before activation",
            )
            data["deployed_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
            self.write_receipt(data, "applying")
            temporary = self.watch / ".scoped-food-release.incoming"
            temporary.symlink_to(self.record.parent)
            os.replace(temporary, self.marker)
            self.durable_directory(self.watch)
            try:
                self.compose(
                    self.runtime,
                    self.o.sha,
                    self.o.sha,
                    data["deployed_at"],
                    "up",
                    "-d",
                    "--no-deps",
                    *SERVICES,
                )
                self.healthy(data, True)
                self.source()
                self.write_receipt(data, "active")
            except Exception:
                try:
                    self.restore(data)
                except Exception as error:
                    self.write_receipt(data, "rollback_failed")
                    raise ReleaseError(
                        "activation failed and scoped rollback failed: " + str(error)
                    ) from error
                raise
        return "active"

    def rollback(self):
        self.trusted(self.record, private=True)
        data = json.loads(self.record.read_text())
        require(
            data["sha"] == self.o.sha and data["format"] == "henukit-food-scoped-v1",
            "rollback receipt identity mismatch",
        )
        current_helper = digest(Path(__file__))
        recovering_predecessor = data["helper_sha256"] != current_helper
        if recovering_predecessor:
            require(
                (data["sha"], data["helper_sha256"]) == PREDECESSOR_ROLLBACK
                and data["status"] in ("rollback_failed", "rolled_back"),
                "rollback receipt helper identity mismatch",
            )
            data["recovery_helper_sha256"] = current_helper
        require(
            not self.marker.is_symlink() or self.marker.resolve() == self.record.parent,
            "another scoped release owns the marker",
        )
        with self.lock_watcher():
            if data["status"] == "rolled_back":
                self.healthy(data, False)
                if recovering_predecessor:
                    self.write_receipt(data, "rolled_back")
                self.remove_owned_marker()
                return "already_rolled_back"
            try:
                self.restore(data)
            except Exception:
                self.write_receipt(data, "rollback_failed")
                raise
        return "rolled_back"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--sha", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--proof-dir", required=True)
    parser.add_argument("--test-root")
    parser.add_argument("--acknowledge-getwork-deferred", action="store_true", required=True)
    mode = parser.add_mutually_exclusive_group(required=True)
    for name in ("preflight", "execute", "rollback"):
        mode.add_argument("--" + name, action="store_true")
    options = parser.parse_args()
    os.umask(0o077)
    require(
        re.fullmatch("[0-9a-f]{40}", options.sha) and options.run_id.isdigit(), "invalid release identity"
    )
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
    except (
        ReleaseError,
        KeyError,
        OSError,
        ValueError,
        subprocess.TimeoutExpired,
        tarfile.TarError,
    ) as error:
        print("scoped-food-release: " + str(error), file=sys.stderr)
        sys.exit(1)
