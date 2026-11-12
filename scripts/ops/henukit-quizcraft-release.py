#!/usr/bin/env python3
"""QuizCraft database gates inside the original full release owner; no publisher."""

import argparse
import contextlib
import datetime
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import select
import shutil
import time
from urllib.parse import urlsplit, urlunsplit
import stat
import subprocess
import sys
import tarfile
import tempfile

CAPABILITY = "english-training-data-v1"
CAP_FILE = "release-gates/english-training-capabilities.json"


def require(value, message="QuizCraft release evidence missing or changed"):
    if not value:
        raise ValueError(message)


def strict_json(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            require(key not in result, "duplicate release evidence key")
            result[key] = value
        return result

    return json.loads(raw, object_pairs_hook=pairs)


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def trusted(path, private=False, directory=False):
    anchor = Path(os.environ.get("HENUKIT_TRUST_ANCHOR", "/"))
    require(
        path.is_absolute() and ".." not in path.parts and path.is_relative_to(anchor)
    )
    for item in (path, *path.parents):
        info = item.lstat()
        require(
            info.st_uid == os.geteuid()
            and not info.st_mode & 0o022
            and not stat.S_ISLNK(info.st_mode)
        )
        require(
            stat.S_ISDIR(info.st_mode)
            if item != path or directory
            else stat.S_ISREG(info.st_mode)
        )
        if item == anchor:
            break
    if private:
        require(stat.S_IMODE(path.stat().st_mode) in (0o400, 0o600))


def identity(path):
    trusted(path)
    info = path.stat()
    result = {
        k: getattr(info, "st_" + k)
        for k in ("dev", "ino", "uid", "gid", "mode", "size", "mtime_ns", "ctime_ns")
    }
    result["sha256"] = digest(path)
    require(
        all(
            getattr(path.stat(), "st_" + k) == result[k]
            for k in result
            if k != "sha256"
        )
    )
    return result


def archive_file(archive, name):
    trusted(archive)
    with tarfile.open(archive, "r:gz") as stream:
        entries = [m for m in stream.getmembers() if m.name.removeprefix("./") == name]
        require(
            len(entries) == 1 and entries[0].isfile(),
            "verified runtime member missing or duplicated",
        )
        return stream.extractfile(entries[0]).read()


def capability(runtime, sha, archive):
    path = runtime / CAP_FILE
    trusted(path)
    require(
        path.read_bytes() == archive_file(archive, CAP_FILE),
        "capability differs from verified artifact",
    )
    value = strict_json(path.read_bytes())
    require(
        value
        == {
            "format": "henukit-data-consumer-capabilities-v1",
            "release_sha": sha,
            "consumers": {
                s: [CAPABILITY] for s in ("quizcraft", "portal-gateway", "portal")
            },
        },
        "incompatible data consumers",
    )
    return identity(path)


def native_database_module(args):
    module = Path(__file__).with_name("henukit-quizcraft-database.py")
    trusted(module)
    spec = importlib.util.spec_from_file_location("quizcraft_native_database", module)
    loaded = importlib.util.module_from_spec(spec)
    source = archive_file(args.archive, "bin/henukit-quizcraft-database.py")
    require(
        module.read_bytes() == source
        and (args.release_dir / "bin/henukit-quizcraft-database.py").read_bytes()
        == source,
        "installed native database module differs from exact candidate",
    )
    require(
        Path(__file__).read_bytes()
        == archive_file(args.archive, "bin/henukit-quizcraft-release.py"),
        "installed release owner differs from exact candidate",
    )
    loaded._loaded_identity = identity(module)
    exec(compile(source, str(module), "exec"), loaded.__dict__)
    require(
        identity(module) == loaded._loaded_identity,
        "installed database module changed during import",
    )
    return loaded


class FullRelease:
    require = staticmethod(require)
    trusted = staticmethod(trusted)

    def __init__(self, args):
        self.o = args
        self.prefix = Path(os.environ.get("HENUKIT_TRUST_ANCHOR", "/"))
        self.testing = self.prefix != Path("/")
        if self.testing:
            require(os.geteuid() != 0, "test authority is unavailable to root")
            require(
                self.prefix.is_relative_to(Path(tempfile.gettempdir()).resolve())
                and self.prefix != Path(tempfile.gettempdir()).resolve()
            )
            require(
                all(
                    p.is_relative_to(self.prefix)
                    for p in (
                        args.state_root,
                        args.release_dir,
                        args.env_file,
                        args.backup_root,
                    )
                )
            )
        else:
            require(
                os.geteuid() == 0
                and os.uname().sysname == "Linux"
                and os.uname().machine == "x86_64"
            )
        trusted(args.env_file, private=True)
        self.env = {
            k: v
            for k, v in os.environ.items()
            if not k.startswith(("PG", "QUIZCRAFT_"))
        }
        self.tools = {}
        for name in ("docker",):
            path = (
                self.prefix / "bin" / name
                if self.testing
                else Path(shutil.which(name) or "")
            )
            trusted(path)
            self.tools[name] = str(path)
        self.init_quizcraft_tools()
        source_lock = Path(
            os.environ.get(
                "HENUKIT_SOURCE_DEPLOY_LOCK", "/run/lock/henukit-deploy.lock"
            )
        )
        for descriptor, path in (
            (6, args.state_root),
            (7, source_lock),
            (9, args.state_root / "watcher.lock"),
        ):
            trusted(path, directory=descriptor == 6)
            opened = os.fstat(descriptor)
            current = path.stat()
            require(
                (opened.st_dev, opened.st_ino) == (current.st_dev, current.st_ino),
                "original release lock unavailable",
            )
            fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
        self.folder = args.state_root / "quizcraft-releases" / args.sha
        self.record = self.folder / "identity.json"

    def path(self, path):
        return self.prefix / str(path).lstrip("/")

    def run(self, name, *args, env=None, input=None, timeout=60):
        result = subprocess.run(
            [self.tools[name], *map(str, args)],
            env=self.env if env is None else env,
            input=input,
            capture_output=True,
            timeout=timeout,
        )
        require(result.returncode == 0, "QuizCraft external command failed")
        return result.stdout

    @staticmethod
    def container_environment(item):
        result = {}
        for value in item["Config"]["Env"]:
            key, sep, text = value.partition("=")
            require(sep and key not in result)
            result[key] = text
        return result

    def containers(self):
        ids = self.run("docker", "ps", "-aq").decode().split()
        require(ids)
        return json.loads(self.run("docker", "inspect", *ids))

    def writer(self):
        items = [
            i for i in self.containers() if i.get("Name") == "/henukit-quizcraft-1"
        ]
        require(len(items) == 1, "sole QuizCraft container unavailable")
        return items[0]

    @staticmethod
    def normalized(value):
        result = dict(value)
        result["started"] = (
            datetime.datetime.fromisoformat(result["started"].replace("Z", "+00:00"))
            .astimezone(datetime.timezone.utc)
            .isoformat()
        )
        return result

    def routed_owner(self, item):
        raw = self.container_environment(item).get("DATABASE_URL", "")
        target = urlsplit(raw)
        require(
            target.scheme in ("postgres", "postgresql")
            and target.hostname == "host.docker.internal"
            and target.port in (None, 5432)
            and target.path == "/quizcraft_v2"
            and target.username
            and target.password
            and target.query in ("", "sslmode=disable")
            and not target.fragment
        )
        require(target.netloc.count("host.docker.internal") == 1)
        self.database_url = urlunsplit(
            (
                target.scheme,
                target.netloc.replace("host.docker.internal", "127.0.0.1"),
                target.path,
                target.query,
                "",
            )
        )
        return hashlib.sha256(raw.encode()).hexdigest()

    def verify_programs(self, payload):
        module = Path(self.native_module.__file__)
        require(
            identity(module) == self.native_module._loaded_identity,
            "loaded native database authority changed",
        )
        require(
            digest(module) == payload["bin/henukit-quizcraft-database.py"]["sha256"],
            "installed native database authority differs from exact candidate",
        )
        require(
            digest(Path(__file__))
            == payload["bin/henukit-quizcraft-release.py"]["sha256"],
            "installed release owner differs from exact candidate",
        )
        return {"owner": identity(Path(__file__)), "database": identity(module)}

    def payload(self):
        names = [
            "bin/quizcraft-release-state",
            "bin/quizcraft-migrate",
            "bin/henukit-quizcraft-release.py",
            "bin/henukit-quizcraft-database.py",
            "release-gates/quizcraft-backup-000011.sql",
            "release-gates/quizcraft-backup-000012.sql",
        ]
        require((self.o.release_dir / "RELEASE_SHA").read_text().strip() == self.o.sha)
        migration_dir = self.o.release_dir / "migrations/quizcraft"
        trusted(migration_dir, directory=True)
        migrations = {p.name for p in migration_dir.iterdir()}
        require(
            len(migrations) == 24
            and all(
                re.fullmatch(r"0000(?:0[1-9]|1[012])_[a-z0-9_]+\.(up|down)\.sql", name)
                for name in migrations
            ),
            "unknown packaged Core migrations",
        )
        names += ["migrations/quizcraft/" + name for name in sorted(migrations)]
        with tarfile.open(self.o.archive, "r:gz") as archive:
            archived = {
                m.name.removeprefix("./")
                for m in archive.getmembers()
                if m.name.removeprefix("./").startswith("migrations/quizcraft/")
                and m.isfile()
            }
        require(archived == {"migrations/quizcraft/" + name for name in migrations})
        result = {}
        for name in names:
            path = self.o.release_dir / name
            trusted(path)
            require(
                path.read_bytes() == archive_file(self.o.archive, name),
                "Core executable or verifier differs from verified runtime",
            )
            result[name] = identity(path)
        require(
            digest(Path(__file__))
            == result["bin/henukit-quizcraft-release.py"]["sha256"],
            "installed release owner differs from exact candidate",
        )
        result[CAP_FILE] = capability(self.o.release_dir, self.o.sha, self.o.archive)
        self.verify_programs(result)
        return result

    def probe(self, snapshot=""):
        path = self.o.release_dir / "bin/quizcraft-release-state"
        args = [
            str(path),
            "--directory",
            str(self.o.release_dir / "migrations/quizcraft"),
        ]
        if snapshot:
            args += ["--snapshot", snapshot]
        env = {**self.env, "QUIZCRAFT_V2_DATABASE_URL": self.database_url}
        result = subprocess.run(args, env=env, capture_output=True, timeout=45)
        require(result.returncode == 0, "Core release state is unknown")
        value = strict_json(result.stdout)
        require(
            set(value)
            == {
                "format",
                "database_identity",
                "schema",
                "requires_english_training_data_v1",
            }
            and value["format"] == "quizcraft-release-state-v1"
            and type(value["requires_english_training_data_v1"]) is bool
        )
        require(value["schema"]["version"] in ("000011", "000012"))
        require(
            self.normalized(value["database_identity"])
            == self.normalized(self.database_identity),
            "Core probe database identity changed",
        )
        return value

    def previous_capability(self):
        require(
            re.fullmatch("[0-9a-f]{40}", self.o.previous_sha or "")
            and self.o.previous_sha != self.o.sha
        )
        previous = self.o.release_dir.parent / self.o.previous_sha
        trusted(previous / "RELEASE_SHA")
        require((previous / "RELEASE_SHA").read_text().strip() == self.o.previous_sha)
        path = previous / CAP_FILE
        if path.exists() or path.is_symlink():
            receipt = (
                self.o.state_root
                / "quizcraft-releases"
                / self.o.previous_sha
                / "identity.json"
            )
            trusted(receipt, private=True)
            original = strict_json(receipt.read_bytes())
            require(
                original["format"] == "henukit-quizcraft-release-v1"
                and original["sha"] == self.o.previous_sha
            )
            meta = Path(original["metadata"])
            trusted(meta, private=True)
            require(
                [meta.stat().st_dev, meta.stat().st_ino] == original["metadata_inode"]
            )
            lines = meta.read_text().splitlines()
            require(
                lines.count(
                    "quizcraft_identity="
                    + json.dumps(
                        identity(receipt), sort_keys=True, separators=(",", ":")
                    )
                )
                == 1
            )
            accepted = receipt.parent / "accepted.json"
            trusted(accepted, private=True)
            require(
                lines.count(
                    "quizcraft_accepted="
                    + json.dumps(
                        identity(accepted), sort_keys=True, separators=(",", ":")
                    )
                )
                == 1
            )
            archive = Path(original["archive_path"])
            require(identity(archive) == original["archive_identity"])
            proof = capability(previous, self.o.previous_sha, archive)
            accepted_value = strict_json(accepted.read_bytes())
            require(
                accepted_value["state"]["schema"]["version"] == "000012"
                and accepted_value["consumer_capability"] == proof
            )
            require(proof == original["payload"][CAP_FILE])
            return proof
        # A previously training-capable release cannot downgrade to legacy by
        # deleting its marker. Original accepted receipt/binary are evidence.
        require(
            not (previous / "bin/quizcraft-release-state").exists()
            and not (
                self.o.state_root / "quizcraft-releases" / self.o.previous_sha
            ).exists(),
            "previous capability evidence disappeared",
        )
        return None

    def compatibility(self, state, previous):
        require(
            not state["requires_english_training_data_v1"] or previous is not None,
            "training facts require compatible immediate-previous consumers",
        )

    @staticmethod
    def durable(path, value):
        require(
            not path.exists() and not path.is_symlink(),
            "attempt evidence already exists",
        )
        with path.open("x") as stream:
            json.dump(value, stream, sort_keys=True, separators=(",", ":"))
            stream.flush()
            os.fsync(stream.fileno())
        path.chmod(0o400)
        descriptor = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)

    def metadata(self):
        marker = self.o.state_root / "prepared" / self.o.sha
        trusted(marker, private=True)
        backup = Path(marker.read_text().strip())
        require(backup.is_relative_to(self.o.backup_root))
        trusted(backup, private=True)
        meta = Path(str(backup) + ".meta")
        trusted(meta, private=True)
        require(meta.read_text().splitlines().count("release_sha=" + self.o.sha) == 1)
        return meta

    def anchor(self, name, path):
        meta = self.metadata()
        with meta.open("a") as stream:
            stream.write(
                "quizcraft_"
                + name
                + "="
                + json.dumps(identity(path), sort_keys=True, separators=(",", ":"))
                + "\n"
            )
            stream.flush()
            os.fsync(stream.fileno())

    def begin(self):
        if self.record.exists():
            return self.load()
        payload = self.payload()
        item = self.writer()
        self.bind_quizcraft_database(item)
        state = self.probe()
        previous = self.previous_capability()
        self.compatibility(state, previous)
        contract = self.o.state_root / "rollback-contracts/pending" / self.o.sha
        trusted(contract, private=True)
        lines = contract.read_text().splitlines()
        require(
            "candidate_sha=" + self.o.sha in lines
            and "previous_sha=" + self.o.previous_sha in lines
            and len(lines) == 8
        )
        approval = self.o.state_root / "approvals" / self.o.sha
        trusted(approval, private=True)
        require(approval.read_text().strip() == self.o.sha)
        self.folder.mkdir(parents=True, mode=0o700)
        trusted(self.folder, directory=True)
        meta = self.metadata()
        info = meta.stat()
        data = {
            "format": "henukit-quizcraft-release-v1",
            "sha": self.o.sha,
            "previous_sha": self.o.previous_sha,
            "programs": self.verify_programs(payload),
            "environment": identity(self.o.env_file),
            "payload": payload,
            "archive_path": str(self.o.archive),
            "archive_identity": identity(self.o.archive),
            "previous_capability": previous,
            "rollback_contract": identity(contract),
            "approval": identity(approval),
            "metadata": str(meta),
            "metadata_inode": [info.st_dev, info.st_ino],
            "metadata_prefix": hashlib.sha256(meta.read_bytes()).hexdigest(),
            "database_identity": self.database_identity,
            "database_url_sha256": self.routed_owner(item),
            "writer_id": item["Id"],
            "writer_image": item["Image"],
            "writer_started": item["State"]["StartedAt"],
            "writer_restarts": item["RestartCount"],
            "writer_environment_sha256": hashlib.sha256(
                json.dumps(self.container_environment(item), sort_keys=True).encode()
            ).hexdigest(),
        }
        self.durable(self.record, data)
        self.anchor("identity", self.record)
        return data

    def load(self):
        trusted(self.record, private=True)
        data = strict_json(self.record.read_bytes())
        require(
            data["format"] == "henukit-quizcraft-release-v1"
            and data["sha"] == self.o.sha
            and data["previous_sha"] == self.o.previous_sha
        )
        require(
            self.verify_programs(data["payload"]) == data["programs"],
            "executed release programs changed",
        )
        meta = self.metadata()
        info = meta.stat()
        require(
            str(meta) == data["metadata"]
            and [info.st_dev, info.st_ino] == data["metadata_inode"]
        )
        raw = meta.read_text()
        prefix, sep, rest = raw.partition("quizcraft_identity=")
        require(sep)
        require(hashlib.sha256(prefix.encode()).hexdigest() == data["metadata_prefix"])
        entries = {}
        for line in ("quizcraft_identity=" + rest).splitlines():
            key, sep, value = line.partition("=")
            require(sep and key not in entries)
            entries[key] = strict_json(value)
        require(entries.get("quizcraft_identity") == identity(self.record))
        allowed = {
            "identity",
            "freeze-intent",
            "backup",
            "migrated",
            "accepted",
            "rollback-freeze-intent",
            "rollback-ready",
        }
        require(all(key.removeprefix("quizcraft_") in allowed for key in entries))
        require(
            {p.stem for p in self.folder.glob("*.json")}
            == {key.removeprefix("quizcraft_") for key in entries},
            "unanchored or missing phase evidence",
        )
        for key, value in entries.items():
            if key == "quizcraft_identity":
                continue
            path = self.folder / (key.removeprefix("quizcraft_") + ".json")
            require(identity(path) == value, "phase receipt missing or replaced")
        contract = self.o.state_root / "rollback-contracts/pending" / self.o.sha
        require(
            identity(contract) == data["rollback_contract"],
            "original rollback contract changed",
        )
        require(
            identity(self.o.archive) == data["archive_identity"],
            "original artifact changed",
        )
        require(
            identity(self.o.env_file) == data["environment"],
            "original environment replaced or changed",
        )
        for name, value in data["payload"].items():
            require(
                identity(self.o.release_dir / name) == value,
                "candidate payload changed",
            )
        self.database_identity = data["database_identity"]
        require(
            self.previous_capability() == data["previous_capability"],
            "previous capability changed",
        )
        backup_event = self.folder / "backup.json"
        if backup_event.exists():
            backup = strict_json(backup_event.read_bytes())
            dump = Path(backup["path"])
            proof = Path(backup["proof_path"])
            require(
                dump.is_relative_to(self.o.backup_root)
                and dump.name == "quizcraft.dump"
                and proof == dump.parent / "proof.json"
            )
            require(
                identity(dump) == backup["backup"]
                and identity(proof) == backup["proof_identity"],
                "retained native backup or proof missing or replaced",
            )
            require(
                strict_json(proof.read_bytes())
                == {
                    k: v
                    for k, v in backup.items()
                    if k not in ("proof_path", "proof_identity")
                }
            )
        self.database_identity = data["database_identity"]
        return data

    def event(self, name, value):
        path = self.folder / (name + ".json")
        if path.exists():
            self.load()
            return strict_json(path.read_bytes())
        self.durable(path, value)
        self.anchor(name, path)
        return value

    def check_database_identity(self):
        for value in (
            self.psql(self.native_module.IDENTITY, url=self.database_url),
            self.psql(self.native_module.IDENTITY, database="quizcraft_v2"),
        ):
            require(
                self.normalized(value) == self.normalized(self.database_identity),
                "native database identity changed during release window",
            )

    def assert_frozen(self, data, rollback=False):
        item = self.writer()
        intent = self.folder / (
            "rollback-freeze-intent.json" if rollback else "freeze-intent.json"
        )
        require(intent.exists(), "missing writer freeze identity")
        frozen = strict_json(intent.read_bytes())
        require(
            item["State"]["Running"] is False
            and not item["State"].get("Restarting", False)
            and not item["State"].get("Paused", False)
        )
        require(
            {
                "container_id": item["Id"],
                "image": item["Image"],
                "started": item["State"]["StartedAt"],
                "restarts": item["RestartCount"],
            }
            == frozen,
            "frozen Core writer restarted or was replaced",
        )
        require(self.routed_owner(item) == data["database_url_sha256"])
        self.check_database_identity()
        self.idle()

    def idle(self):
        value = self.psql(
            "SELECT json_build_object('other_backends',count(*)) FROM pg_stat_activity WHERE datname='quizcraft_v2' AND pid<>pg_backend_pid()",
            database="quizcraft_v2",
        )
        require(
            value == {"other_backends": 0},
            "unexpected Core database writer or connection",
        )

    def freeze(self, data, rollback=False):
        item = self.writer()
        url = self.routed_owner(item)
        require(
            url == data["database_url_sha256"], "existing database authority changed"
        )
        name = "rollback-freeze-intent" if rollback else "freeze-intent"
        if not rollback:
            require(
                item["Id"] == data["writer_id"]
                and item["Image"] == data["writer_image"]
                and item["State"]["StartedAt"] == data["writer_started"]
                and item["RestartCount"] == data["writer_restarts"]
            )
            require(
                hashlib.sha256(
                    json.dumps(
                        self.container_environment(item), sort_keys=True
                    ).encode()
                ).hexdigest()
                == data["writer_environment_sha256"]
            )
        if item["State"]["Running"]:
            self.bind_quizcraft_database(item)
            require(
                self.normalized(self.database_identity)
                == self.normalized(data["database_identity"])
            )
        else:
            captured = self.folder / (name + ".json")
            if rollback and not captured.exists():
                captured = self.folder / "freeze-intent.json"
            require(
                captured.exists(), "stopped writer without captured maintenance intent"
            )
            intent = strict_json(captured.read_bytes())
            require(
                intent["container_id"] == item["Id"]
                and intent["image"] == item["Image"]
                and intent["started"] == item["State"]["StartedAt"]
                and intent["restarts"] == item["RestartCount"]
            )
        for other in self.containers():
            if other["Id"] == item["Id"] or not other["State"]["Running"]:
                continue
            for key, value in self.container_environment(other).items():
                if key.endswith("DATABASE_URL"):
                    parsed = urlsplit(value)
                    require(
                        not (
                            parsed.path == "/quizcraft_v2"
                            and parsed.hostname
                            in ("host.docker.internal", "127.0.0.1", "localhost")
                        ),
                        "multiple active Core writers",
                    )
        self.event(
            name,
            {
                "container_id": item["Id"],
                "image": item["Image"],
                "started": item["State"]["StartedAt"],
                "restarts": item["RestartCount"],
            },
        )
        if item["State"]["Running"]:
            self.run("docker", "stop", item["Id"], timeout=90)
        stopped = self.writer()
        require(
            stopped["Id"] == item["Id"]
            and stopped["State"]["Running"] is False
            and not stopped["State"].get("Restarting", False),
            "Core writer did not stop",
        )
        self.check_database_identity()
        self.idle()
        return self.probe()

    @contextlib.contextmanager
    def snapshot(self):
        process = subprocess.Popen(
            [
                self.tools["psql"],
                "-X",
                "--no-password",
                "-qAt",
                "-v",
                "ON_ERROR_STOP=1",
            ],
            env=self.pg_environment(self.database_url),
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        try:
            process.stdin.write(
                b"BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY; SELECT json_build_object('snapshot',pg_export_snapshot());\n"
            )
            process.stdin.flush()
            require(
                select.select([process.stdout], [], [], 20)[0],
                "snapshot export timeout",
            )
            value = strict_json(process.stdout.readline())
            require(re.fullmatch("[0-9A-F]+-[0-9A-F]+-[0-9]+", value["snapshot"]))
            yield value["snapshot"]
            process.stdin.write(b"ROLLBACK;\n\\q\n")
            process.stdin.flush()
            process.communicate(timeout=15)
            require(process.returncode == 0)
        finally:
            if process.poll() is None:
                process.kill()
            process.communicate()

    def content(self, sql, snapshot="", database=None):
        # Only Core's exact artifact-owned SELECT is accepted. This wrapper owns
        # transaction/snapshot transport, never table selection or business SQL.
        query = "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;"
        if snapshot:
            require(re.fullmatch("[0-9A-F]+-[0-9A-F]+-[0-9]+", snapshot))
            query += "SET TRANSACTION SNAPSHOT '" + snapshot + "';"
        query += (
            "SET LOCAL search_path=public,pg_catalog; SET LOCAL timezone='UTC'; SET LOCAL DateStyle='ISO, YMD'; SET LOCAL intervalstyle='postgres'; SET LOCAL extra_float_digits=3; SET LOCAL bytea_output='hex'; COPY ("
            + sql
            + ") TO STDOUT; ROLLBACK;"
        )
        args = [
            self.tools["psql"],
            "-X",
            "--no-password",
            "-qAt",
            "-v",
            "ON_ERROR_STOP=1",
        ]
        if database:
            args = [
                self.tools["runuser"],
                "-u",
                "postgres",
                "--",
                *args,
                "--dbname",
                database,
            ]
        process = subprocess.Popen(
            args,
            env=self.pg_environment(None if database else self.database_url),
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        result = {}
        last = None
        deadline = time.monotonic() + 180
        try:
            process.stdin.write(query.encode())
            process.stdin.close()
            process.stdin = None
            while True:
                require(
                    time.monotonic() < deadline
                    and select.select([process.stdout], [], [], 20)[0],
                    "backup content verification timeout",
                )
                line = process.stdout.readline(512)
                if not line:
                    break
                require(line.endswith(b"\n") and len(line) < 512)
                table, sep, value = line.rstrip(b"\n").partition(b"\t")
                require(sep)
                table = table.decode()
                value = value.decode()
                require(
                    re.fullmatch("quizcraft_[a-z_]+|@catalog", table)
                    and (value == "" or re.fullmatch("[0-9a-f]{64}", value))
                )
                key = (table, value)
                require(last is None or key >= last)
                last = key
                if value == "":
                    require(table not in result)
                    result[table] = {"rows": 0, "hasher": hashlib.sha256()}
                else:
                    require(table in result)
                    result[table]["rows"] += 1
                    result[table]["hasher"].update((value + "\n").encode())
            process.communicate(timeout=15)
            require(process.returncode == 0, "Core backup content verification failed")
            require("@catalog" in result and "quizcraft_schema_migrations" in result)
            return {
                k: {"rows": v["rows"], "sha256": v["hasher"].hexdigest()}
                for k, v in result.items()
            }
        finally:
            if process.poll() is None:
                process.kill()
            process.communicate()

    def backup(self, state):
        self.check_database_identity()
        folder = self.o.backup_root / (
            "quizcraft-" + self.o.sha + "-" + secrets.token_hex(12)
        )
        folder.mkdir(mode=0o700)
        trusted(folder, directory=True)
        archive = folder / "quizcraft.dump"
        version = state["schema"]["version"]
        path = self.o.release_dir / (
            "release-gates/quizcraft-backup-" + version + ".sql"
        )
        trusted(path)
        sql = path.read_text().strip()
        with self.snapshot() as snapshot:
            snap_state = self.probe(snapshot)
            require(snap_state["schema"] == state["schema"])
            content = self.content(sql, snapshot)
            self.run(
                "pg_dump",
                "--format=custom",
                "--snapshot",
                snapshot,
                "--file",
                archive,
                env=self.pg_environment(self.database_url),
                timeout=180,
            )
        archive.chmod(0o400)
        trusted(archive, private=True)
        with archive.open("rb") as stream:
            os.fsync(stream.fileno())
        require(
            archive.stat().st_size > 0 and self.run("pg_restore", "--list", archive)
        )
        database = (
            "henukit_quizcraft_verify_" + self.o.sha[:8] + "_" + secrets.token_hex(12)
        )
        self.check_database_identity()
        self.run(
            "runuser",
            "-u",
            "postgres",
            "--",
            self.tools["createdb"],
            "--template",
            "template0",
            "--",
            database,
            env=self.pg_environment(),
        )
        try:
            with archive.open("rb") as stream:
                result = subprocess.run(
                    [
                        self.tools["runuser"],
                        "-u",
                        "postgres",
                        "--",
                        self.tools["pg_restore"],
                        "--exit-on-error",
                        "--no-owner",
                        "--no-privileges",
                        "--dbname",
                        database,
                    ],
                    env=self.pg_environment(),
                    stdin=stream,
                    capture_output=True,
                    timeout=180,
                )
            require(result.returncode == 0, "isolated Core restore failed")
            self.check_database_identity()
            require(
                self.content(sql, database=database) == content,
                "isolated Core content or catalog differs",
            )
        finally:
            self.run(
                "runuser",
                "-u",
                "postgres",
                "--",
                self.tools["dropdb"],
                "--",
                database,
                env=self.pg_environment(),
            )
        self.check_database_identity()
        proof = {
            "database_identity": self.database_identity,
            "schema": state["schema"],
            "backup": identity(archive),
            "path": str(archive),
            "content": content,
        }
        self.durable(folder / "proof.json", proof)
        return {
            **proof,
            "proof_path": str(folder / "proof.json"),
            "proof_identity": identity(folder / "proof.json"),
        }

    def prepare(self):
        self.payload()
        item = self.writer()
        self.bind_quizcraft_database(item)
        state = self.probe()
        previous = self.previous_capability()
        self.compatibility(state, previous)
        return self.backup(state)

    def migrate(self):
        data = self.load()
        state = self.freeze(data)
        if (self.folder / "migrated.json").exists():
            require(state["schema"]["version"] == "000012")
            return
        proof = self.backup(state)
        self.event("backup", proof)
        self.assert_frozen(data)
        result = subprocess.run(
            [
                str(self.o.release_dir / "bin/quizcraft-migrate"),
                "--directory",
                str(self.o.release_dir / "migrations/quizcraft"),
            ],
            env={**self.env, "QUIZCRAFT_V2_DATABASE_URL": self.database_url},
            capture_output=True,
            timeout=180,
        )
        require(result.returncode == 0, "Core versioned migration failed")
        state = self.probe()
        require(state["schema"]["version"] == "000012")
        self.compatibility(state, data["previous_capability"])
        self.event(
            "migrated", {"state": state, "migration_report": strict_json(result.stdout)}
        )

    def guard(self, rollback=False):
        data = self.load()
        require(
            (
                self.folder / ("rollback-ready.json" if rollback else "migrated.json")
            ).exists(),
            "missing Core migration window",
        )
        if self.o.activation_target:
            require(
                self.o.activation_target
                == (data["previous_sha"] if rollback else data["sha"])
            )
        self.assert_frozen(data, rollback)
        state = self.probe()
        if not rollback:
            require(state["schema"]["version"] == "000012")
        self.compatibility(state, data["previous_capability"])

    def guard_rollback(self):
        require(
            self.o.rollback_env_file is not None,
            "original rollback environment is required",
        )
        trusted(self.o.rollback_env_file, private=True)
        contract = self.o.state_root / "rollback-contracts/pending" / self.o.sha
        trusted(contract, private=True)
        expected = [
            line.partition("=")[2]
            for line in contract.read_text().splitlines()
            if line.startswith("rollback_env_sha256=")
        ]
        require(
            expected == [digest(self.o.rollback_env_file)],
            "rollback input differs from original contract",
        )
        return self.guard(rollback=True)

    def accept(self):
        data = self.load()
        item = self.writer()
        self.bind_quizcraft_database(item)
        require(self.routed_owner(item) == data["database_url_sha256"])
        state = self.probe()
        require(state["schema"]["version"] == "000012")
        self.event(
            "accepted",
            {"state": state, "consumer_capability": data["payload"][CAP_FILE]},
        )

    def rollback(self):
        data = self.load()
        state = self.freeze(data, rollback=True)
        self.compatibility(state, data["previous_capability"])
        self.event("rollback-ready", {"state": state})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "action",
        choices=[
            "capability",
            "prepare",
            "begin",
            "migrate",
            "guard",
            "guard-rollback",
            "accept",
            "rollback",
        ],
    )
    parser.add_argument("--sha", required=True)
    parser.add_argument("--release-dir", type=Path, required=True)
    parser.add_argument("--archive", type=Path, required=True)
    parser.add_argument("--previous-sha")
    parser.add_argument("--activation-target")
    parser.add_argument("--rollback-env-file", type=Path)
    parser.add_argument("--state-root", type=Path)
    parser.add_argument("--env-file", type=Path)
    parser.add_argument("--backup-root", type=Path)
    args = parser.parse_args()
    require(re.fullmatch("[0-9a-f]{40}", args.sha))
    os.umask(0o077)
    if args.action == "capability":
        proof = capability(args.release_dir, args.sha, args.archive)
        print(
            json.dumps(
                {"training_compatible": True, "capability_sha256": proof["sha256"]}
            )
        )
        return
    else:
        require(
            all((args.previous_sha, args.state_root, args.env_file, args.backup_root))
        )
        native = native_database_module(args)

        class VerifiedFullRelease(FullRelease, native.QuizcraftDatabase):
            native_module = native

        owner = VerifiedFullRelease(args)
        result = getattr(owner, args.action.replace("-", "_"))()
    print(json.dumps({"status": args.action + "_verified", "evidence": result}))


if __name__ == "__main__":
    try:
        main()
    except (
        ValueError,
        OSError,
        KeyError,
        TypeError,
        subprocess.SubprocessError,
        tarfile.TarError,
    ):
        print(
            "QuizCraft release gate could not be verified; retain maintenance state for operator recovery",
            file=sys.stderr,
        )
        sys.exit(1)
