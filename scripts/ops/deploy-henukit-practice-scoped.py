#!/usr/bin/env python3
"""Release the fixed Practice/Food dependency cohort; getWork remains deferred."""

import argparse
import contextlib
import fcntl
import hashlib
import importlib.util
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import select
import stat
import subprocess
import sys
import tarfile
import tempfile
from urllib.parse import parse_qsl, unquote, urlsplit, urlunsplit

BASE = Path(__file__).with_name("deploy-henukit-food-scoped.py")
# Validate the import before executing it. Provenance later binds both files to
# the authenticated runtime; the operator bootstrap must verify before invoking.
require_owner = os.geteuid()
import_boundary = Path("/")
if "--test-root" in sys.argv:
    import_boundary = Path(sys.argv[sys.argv.index("--test-root") + 1]).resolve()
    if require_owner == 0 or not import_boundary.is_relative_to(Path(tempfile.gettempdir()).resolve()) or import_boundary == Path(tempfile.gettempdir()).resolve() or not BASE.resolve().is_relative_to(import_boundary):
        raise SystemExit("scoped-practice-release: invalid test boundary")
for entry in (BASE, *BASE.absolute().parents):
    if entry == import_boundary:
        break
    info = entry.lstat()
    if entry.is_symlink() or info.st_uid != require_owner or info.st_mode & 0o022:
        raise SystemExit("scoped-practice-release: untrusted engine path")
spec = importlib.util.spec_from_file_location("henukit_scoped_engine", BASE)
engine = importlib.util.module_from_spec(spec)
spec.loader.exec_module(engine)
require = engine.require

DB_BASE = Path(__file__).with_name("henukit-quizcraft-database.py")
for entry in (DB_BASE, *DB_BASE.absolute().parents):
    if entry == import_boundary:
        break
    info = entry.lstat()
    if entry.is_symlink() or info.st_uid != require_owner or info.st_mode & 0o022:
        raise SystemExit("scoped-practice-release: untrusted database authority path")
db_spec = importlib.util.spec_from_file_location("henukit_quizcraft_database", DB_BASE)
db_module = importlib.util.module_from_spec(db_spec)
db_source = DB_BASE.read_bytes()
exec(compile(db_source, str(DB_BASE), "exec"), db_module.__dict__)

SERVICES = ("portal", "portal-gateway", "food", "console-gateway", "quizcraft")
CLIENT = "console-gateway-quizcraft-management"
KEY_ID = "quizcraft-console-management-v1"
PLACEHOLDER = "preflight-only-application-key-placeholder-64-characters-xxxxxxxxxx"
MIGRATIONS = "products/quizcraft/go-service/db/migrations"
COUNTS = """SELECT json_build_object(
'banks',(SELECT count(*) FROM quizcraft_banks),
'versions',(SELECT count(*) FROM quizcraft_bank_versions),
'questions',(SELECT count(*) FROM quizcraft_questions),
'question_versions',(SELECT count(*) FROM quizcraft_question_versions),
'memberships',(SELECT count(*) FROM quizcraft_bank_version_questions),
'attempts',(SELECT count(*) FROM quizcraft_practice_attempts),
'sessions',(SELECT count(*) FROM quizcraft_practice_sessions),
'audit',(SELECT count(*) FROM quizcraft_workshop_audit_events))"""
IDENTITY = """SELECT json_build_object('database',current_database(),
'oid',(SELECT oid FROM pg_database WHERE datname=current_database()),
'started',pg_postmaster_start_time(),'port',current_setting('port')::int,
'version',current_setting('server_version_num')::int)"""
# Linux UAPI: NS_GET_NSTYPE and CLONE_NEWNET. A regular file is not a namespace.
NS_GET_NSTYPE = 0xB703
CLONE_NEWNET = 0x40000000


def canonical(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


class Release(db_module.QuizcraftDatabase, engine.Release):
    require = staticmethod(engine.require)
    services = SERVICES
    helper_file = Path(__file__)
    state_scope = "practice-console"
    receipt_format = "henukit-practice-scoped-v1"
    predecessor_rollbacks = ()

    def __init__(self, options):
        super().__init__(options)
        self.application = None
        self.verify_database_module_source()
        self.init_quizcraft_tools()
        # Caller-supplied bridge/PG values are never authority for either side.
        self.env = {k: v for k, v in self.env.items() if not k.startswith(("PG", "QUIZCRAFT_CONSOLE_"))}

    def application_environment(self, secret):
        return {
            "QUIZCRAFT_CONSOLE_ENABLED": "1",
            "QUIZCRAFT_CONSOLE_CLIENT_ID": CLIENT,
            "QUIZCRAFT_CONSOLE_API_URL": "http://quizcraft:10089",
            "QUIZCRAFT_CONSOLE_KEY_ID": KEY_ID,
            "QUIZCRAFT_CONSOLE_SECRET": secret,
            "QUIZCRAFT_CONSOLE_KEYS_JSON": json.dumps({KEY_ID: secret}, separators=(",", ":")),
        }

    def compose(self, runtime, sha, version, deployed_at, *args):
        env = self.env
        if sha == self.o.sha:
            env = {**env, **self.application_environment(self.application or PLACEHOLDER)}
        original = self.env
        self.env = env
        try:
            return super().compose(runtime, sha, version, deployed_at, *args)
        finally:
            self.env = original

    def compatible_configuration(self, baseline, candidate):
        allowed = self.application_environment(PLACEHOLDER)
        actual = json.loads(json.dumps(candidate))
        for service, names in {
            "console-gateway": ("ENABLED", "CLIENT_ID", "API_URL", "KEY_ID", "SECRET"),
            "quizcraft": ("ENABLED", "CLIENT_ID", "KEYS_JSON"),
        }.items():
            before = baseline["services"][service].get("environment", {})
            after = actual["services"][service].get("environment", {})
            for suffix in names:
                key = "QUIZCRAFT_CONSOLE_" + suffix
                if key in before or after.pop(key, None) != allowed[key]:
                    return False
        return baseline == actual

    def verify_database_module_source(self):
        # This local exact-source check also precedes idempotent/offline receipt
        # recovery. Fresh preflight additionally verifies the original attestation.
        with self.safe_tar(self.archive("henukit-runtime-" + self.o.sha + ".tar.gz")) as archive:
            source = archive.extractfile("./bin/henukit-quizcraft-database.py").read()
            require(source == db_source and hashlib.sha256(source).hexdigest() == engine.digest(DB_BASE),
                    "executed native database module differs from attested runtime")

    def provenance(self):
        images = super().provenance()
        self.verify_database_module_source()
        return images

    def database_target(self, containers):
        engine.Release.database_target(self, containers)
        self.bind_quizcraft_database(containers["henukit-quizcraft-1"])

    def migration_inventory(self, sha):
        entries = json.loads(self.run("gh", "api", "repos/" + engine.REPO + "/contents/" + MIGRATIONS + "?ref=" + sha))
        require(isinstance(entries, list) and entries, "missing QuizCraft migration inventory")
        result = {}
        for item in entries:
            name = item["name"]
            require(item["type"] == "file" and re.fullmatch(r"[0-9]{6}_[a-z0-9_]+\.(up|down)\.sql|[0-9]{6}\.(up|down)\.sql", name) and item["path"] == MIGRATIONS + "/" + name and re.fullmatch(r"[0-9a-f]{40}", item["sha"]) and name not in result, "unsafe QuizCraft migration inventory")
            result[name] = item["sha"]
        return result

    def preflight(self):
        data = super().preflight()
        before = self.migration_inventory(data["baseline"])
        after = self.migration_inventory(data["sha"])
        require(before == after, "QuizCraft schema changes require a full migration release")
        folder = self.runtime / "migrations/quizcraft"
        require({p.name for p in folder.iterdir()} == set(after), "QuizCraft migration payload inventory differs")
        for name, blob in after.items():
            path = folder / name
            self.trusted(path)
            content = path.read_bytes()
            require(hashlib.sha1(b"blob " + str(len(content)).encode() + b"\0" + content).hexdigest() == blob, "QuizCraft migration payload differs from exact source")
        # The runtime archive must also contain exactly this migration payload.
        with self.safe_tar(self.archive("henukit-runtime-" + self.o.sha + ".tar.gz")) as archive:
            names = {m.name.removeprefix("./") for m in archive.getmembers() if m.isfile() and m.name.removeprefix("./").startswith("migrations/quizcraft/")}
            require(names == {"migrations/quizcraft/" + n for n in after}, "attested QuizCraft migration inventory differs")
            for name in after:
                require(archive.extractfile("./migrations/quizcraft/" + name).read() == (folder / name).read_bytes(), "attested QuizCraft migration content differs")
        data.update(scope=list(SERVICES), engine_sha256=engine.digest(BASE), database_helper_sha256=engine.digest(DB_BASE), candidate_compose_sha256=engine.digest(self.runtime / "docker-compose.henukit.release.yml"), quizcraft_migrations=after, database_identity=self.database_identity)
        return data

    @contextlib.contextmanager
    def database_snapshot(self):
        process = subprocess.Popen(
            [self.tools["psql"], "--no-psqlrc", "--no-password", "--quiet", "--tuples-only", "--no-align", "--set", "ON_ERROR_STOP=1"],
            env=self.pg_environment(self.database_url), stdin=subprocess.PIPE,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        try:
            query = "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY; SELECT json_build_object('snapshot',pg_export_snapshot(),'counts',(" + COUNTS + "));\n"
            process.stdin.write(query.encode())
            process.stdin.flush()
            require(select.select([process.stdout], [], [], 20)[0], "QuizCraft snapshot export timed out")
            line = process.stdout.readline()
            require(line, "QuizCraft snapshot export failed")
            data = json.loads(line)
            require(re.fullmatch(r"[0-9A-F]+-[0-9A-F]+-[0-9]+", data["snapshot"]), "invalid QuizCraft snapshot identifier")
            yield data
            process.stdin.write(b"ROLLBACK;\n\\q\n")
            process.stdin.flush()
            _, _ = process.communicate(timeout=15)
            require(process.returncode == 0, "QuizCraft snapshot transaction failed")
        finally:
            if process.poll() is None:
                process.kill()
            process.communicate()
            for stream in (process.stdin, process.stdout, process.stderr):
                stream.close()

    def backup(self, data):
        folder = self.record.parent
        backup = folder / "configuration.backup"
        require(not backup.exists() and not backup.is_symlink(), "configuration backup already exists")
        # Reuse the proven Food/container-PostgreSQL backup and isolated restore.
        # It owns the one global configuration copy. Capture its receipt fields
        # before the separate host-PostgreSQL QuizCraft path adds its evidence.
        self.database_target(self.containers())
        food = dict(data)
        engine.Release.backup(self, food)
        data["database_backups"] = {
            "food": {
                "sha256": food["backup_sha256"],
                "bytes": food["backup_bytes"],
                "counts": {"food_posts": food["restored_food_posts"]},
            }
        }
        self.write_receipt(data, "preparing")
        self.database_target(self.containers())
        require(self.database_identity == data["database_identity"], "QuizCraft database identity changed before backup")
        archive = folder / "quizcraft.dump"
        require(not archive.exists(), "database backup already exists")
        with self.database_snapshot() as snapshot:
            self.run("pg_dump", "--format=custom", "--snapshot", snapshot["snapshot"], "--file", archive, env=self.pg_environment(self.database_url), timeout=180)
        self.trusted(archive, private=True)
        with archive.open("rb") as stream:
            os.fsync(stream.fileno())
        self.durable_directory(folder)
        require(archive.stat().st_size > 0 and self.run("pg_restore", "--list", archive), "QuizCraft database backup unreadable")
        # Compare to the exact exported dump snapshot; live student writes
        # after the snapshot are retained and cannot invalidate this comparison.
        database = "henukit_practice_verify_" + self.o.sha[:8] + "_" + secrets.token_hex(12)
        require(re.fullmatch(r"henukit_practice_verify_[0-9a-f]{8}_[0-9a-f]{24}", database), "unsafe verification database")
        self.run("runuser", "-u", "postgres", "--", self.tools["createdb"], "--template", "template0", "--", database, env=self.pg_environment())
        try:
            with archive.open("rb") as stream:
                result = subprocess.run(
                    [self.tools["runuser"], "-u", "postgres", "--", self.tools["pg_restore"], "--exit-on-error", "--no-owner", "--no-privileges", "--dbname", database],
                    env=self.pg_environment(), stdin=stream, capture_output=True, timeout=180,
                )
            require(result.returncode == 0, "isolated QuizCraft database restore failed")
            counts = self.psql(COUNTS, database=database)
            require(counts == snapshot["counts"], "restored QuizCraft snapshot counts differ")
            require(set(counts) == {"banks", "versions", "questions", "question_versions", "memberships", "attempts", "sessions", "audit"} and all(type(v) is int and v >= 0 for v in counts.values()), "restored QuizCraft schema could not be read")
        finally:
            self.run("runuser", "-u", "postgres", "--", self.tools["dropdb"], "--", database, env=self.pg_environment())
        data["database_backups"]["quizcraft"] = {
            "sha256": engine.digest(archive), "bytes": archive.stat().st_size, "counts": counts,
        }
        # Both database restores and their private evidence must be durable
        # before creating the one new application HMAC key.
        self.write_receipt(data, "preparing")
        self.application = secrets.token_urlsafe(48)
        secret_file = folder / "quizcraft-console-application.json"
        with secret_file.open("x") as stream:
            json.dump({"client_id": CLIENT, "key_id": KEY_ID, "secret": self.application}, stream)
            stream.flush()
            os.fsync(stream.fileno())
        secret_file.chmod(0o600)
        self.trusted(secret_file, private=True)
        self.durable_directory(folder)
        data["application_secret_sha256"] = engine.digest(secret_file)

    def load_application(self, data):
        require(data.get("sha") == self.o.sha and data.get("run_id") == self.o.run_id and data.get("format") == self.receipt_format and data.get("scope") == list(SERVICES), "receipt release identity mismatch")
        self.trusted(self.runtime / "docker-compose.henukit.release.yml")
        require(data.get("engine_sha256") == engine.digest(BASE), "receipt engine identity mismatch")
        require(data.get("database_helper_sha256") == engine.digest(DB_BASE), "receipt database authority identity mismatch")
        require(data.get("candidate_compose_sha256") == engine.digest(self.runtime / "docker-compose.henukit.release.yml"), "candidate Compose changed")
        if "application_secret_sha256" in data:
            path = self.record.parent / "quizcraft-console-application.json"
            self.trusted(path, private=True)
            require(engine.digest(path) == data["application_secret_sha256"], "application secret file changed")
            value = json.loads(path.read_text())
            require(value["client_id"] == CLIENT and value["key_id"] == KEY_ID and re.fullmatch(r"[A-Za-z0-9_-]{64}", value["secret"]), "invalid dedicated application key")
            self.application = value["secret"]

    def unchanged(self, data):
        self.trusted(self.envfile, private=True)
        super().unchanged(data)
        self.load_application(data)

    def scope_acceptance(self, data, candidate):
        engine.Release.scope_acceptance(self, data, candidate)
        # Config hashes bind root/resources as well as exact private env values.
        containers = self.containers()
        runtime = self.runtime if candidate else Path(data["baseline_runtime"])
        sha = data["sha"] if candidate else data["baseline"]
        version = sha if candidate else data["old_version"]
        deployed = data["deployed_at"] if candidate else data["old_deployed_at"]
        rendered = json.loads(self.compose(runtime, sha, version, deployed, "config", "--format", "json"))
        self.running_configuration(containers, runtime, sha, version, deployed, rendered)
        if candidate:
            self.http("/api/v1/quizcraft/workshop/catalog", (401,), origin="https://console.henukit.cn")
            self.http("/api/v1/practice/catalog", (200,))
            addresses = {n.get("IPAddress") for n in containers["henukit-quizcraft-1"]["NetworkSettings"]["Networks"].values()}
            require(len(addresses) == 1, "ambiguous QuizCraft owner endpoint")
            address = next(iter(addresses))
            require(ipaddress.ip_address(address).version == 4 and ipaddress.ip_address(address).is_private, "unreviewed QuizCraft owner endpoint")
            self.http("/api/v1/console/workshop/catalog", (401,), origin="http://" + address + ":10089")
        data["effective_configuration_sha256"] = canonical({s: rendered["services"][s] for s in self.services})

    def execute(self):
        if self.record.exists():
            self.trusted(self.record, private=True)
            self.load_application(json.loads(self.record.read_text()))
        return super().execute()

    def rollback(self):
        self.trusted(self.record, private=True)
        self.load_application(json.loads(self.record.read_text()))
        return super().rollback()


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
        # External tool output and connection strings are never included.
        print("scoped-practice-release: " + str(error), file=sys.stderr)
        sys.exit(1)
