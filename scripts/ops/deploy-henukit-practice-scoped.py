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


class Release(engine.Release):
    services = SERVICES
    helper_file = Path(__file__)
    state_scope = "practice-console"
    receipt_format = "henukit-practice-scoped-v1"
    predecessor_rollbacks = ()

    def __init__(self, options):
        super().__init__(options)
        self.application = None
        for name in ("psql", "pg_dump", "pg_restore", "createdb", "dropdb", "runuser"):
            path = self.prefix / "bin" / name if self.testing else Path("/usr/sbin/runuser") if name == "runuser" else Path("/usr/lib/postgresql/16/bin") / name
            self.trusted(path)
            self.tools[name] = str(path)
        namespace_tool = self.prefix / "bin/nsenter" if self.testing else Path("/usr/bin/nsenter")
        self.trusted(namespace_tool)
        self.tools["nsenter"] = str(namespace_tool)
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

    def pg_environment(self, url=None):
        env = {k: v for k, v in self.env.items() if not k.startswith("PG")}
        env["PGCONNECT_TIMEOUT"] = "10"
        if url:
            target = urlsplit(url)
            env.update(PGHOST=target.hostname, PGPORT=str(target.port or 5432),
                       PGDATABASE=unquote(target.path.removeprefix("/")),
                       PGUSER=unquote(target.username or ""), PGPASSWORD=unquote(target.password or ""))
            # The route gate permits only sslmode=disable (or no query).
            # Keep it in libpq's environment without a secret-bearing argv.
            for key, value in parse_qsl(target.query):
                require(key == "sslmode" and value == "disable", "unreviewed PostgreSQL query option")
                env["PGSSLMODE"] = value
        return env

    def psql(self, query, *, url=None, database=None, namespace=None):
        args = ["--no-psqlrc", "--no-password", "--set", "ON_ERROR_STOP=1", "--tuples-only", "--no-align"]
        if database:
            output = self.run("runuser", "-u", "postgres", "--", self.tools["psql"], "--dbname", database, *args, "--command", query, env=self.pg_environment())
        elif namespace is None:
            output = self.run("psql", *args, "--command", query, env=self.pg_environment(url))
        else:
            # Pin the already-open network namespace across exec. No mount,
            # user or PID namespace is entered; the trusted host PG16 tool and
            # exact owner credentials remain the same.
            result = subprocess.run(
                [self.tools["nsenter"], "--net=/proc/self/fd/" + str(namespace), "--",
                 self.tools["psql"], *args, "--command", query],
                env=self.pg_environment(url), pass_fds=(namespace,),
                capture_output=True, timeout=60,
            )
            require(result.returncode == 0, "QuizCraft container-network psql command failed (" + str(result.returncode) + ")")
            output = result.stdout
        return json.loads(output)

    @staticmethod
    def namespace_container_identity(item):
        state = item.get("State", {})
        pid = state.get("Pid")
        require(re.fullmatch(r"[0-9a-f]{64}", item.get("Id", ""))
                and item.get("Name") == "/henukit-quizcraft-1"
                and state.get("Running") is True and not state.get("Paused", False)
                and not state.get("Restarting", False)
                and type(pid) is int and pid > 1 and state.get("StartedAt"),
                "QuizCraft container is not a stable running namespace owner")
        return {"id": item["Id"], "pid": pid, "started": state["StartedAt"],
                "restarts": item["RestartCount"], "hosts": item["HostsPath"],
                "network": item["NetworkSettings"],
                "extra_hosts": item["HostConfig"].get("ExtraHosts"),
                "database_url": Release.container_environment(item).get("DATABASE_URL")}

    @contextlib.contextmanager
    def owner_network_namespace(self, item):
        expected = self.namespace_container_identity(item)
        namespace_path = self.path("/proc/" + str(expected["pid"]) + "/ns/net")
        sandbox = Path(item["NetworkSettings"].get("SandboxKey", ""))
        require(sandbox.parent in {self.path("/run/docker/netns"), self.path("/var/run/docker/netns")}
                and not sandbox.is_symlink(), "unreviewed QuizCraft Docker sandbox path")
        # /var/run may be the system's root-owned /run alias. Validate the
        # resolved retained mount and every parent before using its identity.
        sandbox = sandbox.resolve(strict=True)
        self.trusted(sandbox)
        if self.testing:
            self.trusted(namespace_path)
        else:
            require(namespace_path.is_symlink()
                    and re.fullmatch(r"net:\[[0-9]+\]", os.readlink(namespace_path)),
                    "QuizCraft proc entry is not a kernel network namespace")
        descriptor = os.open(namespace_path, os.O_RDONLY | os.O_CLOEXEC)
        try:
            opened = os.fstat(descriptor)
            require(stat.S_ISREG(opened.st_mode), "QuizCraft network namespace is not a regular namespace handle")
            if not self.testing:
                require(fcntl.ioctl(descriptor, NS_GET_NSTYPE) == CLONE_NEWNET,
                        "QuizCraft handle is not a Linux network namespace")

            def unchanged():
                actual = json.loads(self.run("docker", "inspect", item["Id"]))
                require(len(actual) == 1 and self.namespace_container_identity(actual[0]) == expected,
                        "QuizCraft container changed during namespace binding")
                for path in (namespace_path, sandbox):
                    current = path.stat()
                    require((current.st_dev, current.st_ino) == (opened.st_dev, opened.st_ino),
                            "QuizCraft process and Docker sandbox namespaces differ or changed")

            unchanged()
            yield descriptor
            unchanged()
        finally:
            os.close(descriptor)

    def database_target(self, containers):
        # The new Portal exposes Food author editing, so the retained Food
        # boundary is part of this fixed dependency closure as well.
        engine.Release.database_target(self, containers)
        item = containers["henukit-quizcraft-1"]
        raw = self.container_environment(item).get("DATABASE_URL", "")
        target = urlsplit(raw)
        require(
            target.scheme in ("postgres", "postgresql")
            and target.hostname == "host.docker.internal" and target.port in (None, 5432)
            and target.path == "/quizcraft_v2" and target.username and target.password
            and not target.fragment and target.query in ("", "sslmode=disable"),
            "QuizCraft target is not the reviewed host PostgreSQL database",
        )
        require("host.docker.internal:host-gateway" in item.get("HostConfig", {}).get("ExtraHosts", []), "QuizCraft host route is not Docker host-gateway")
        # Docker's host-gateway may resolve to its default bridge, rather than
        # this container's Compose network gateway. Read the kernel-managed
        # container hosts file which Docker inspect binds to this exact ID.
        require(re.fullmatch(r"[0-9a-f]{64}", item["Id"]), "invalid QuizCraft container identity")
        hosts = self.path("/var/lib/docker/containers/" + item["Id"] + "/hosts")
        require(item.get("HostsPath") == str(hosts), "QuizCraft hosts path differs from container identity")
        self.trusted(hosts)
        addresses = set()
        for line in hosts.read_text().splitlines():
            fields = line.partition("#")[0].split()
            if len(fields) > 1 and "host.docker.internal" in fields[1:]:
                addresses.add(fields[0])
        require(len(addresses) == 1, "ambiguous QuizCraft host-gateway resolution")
        gateway = next(iter(addresses))
        require(ipaddress.ip_address(gateway).version == 4 and ipaddress.ip_address(gateway).is_private, "unreviewed QuizCraft gateway address")
        # Preserve the exact scheme, credentials, port spelling, DB and query.
        authority = target.netloc
        require(authority.count("host.docker.internal") == 1, "ambiguous database authority")
        def routed(host):
            return urlunsplit((target.scheme, authority.replace("host.docker.internal", host), target.path, target.query, ""))
        self.database_url = routed("127.0.0.1")
        host_identity = self.psql(IDENTITY, url=self.database_url)
        with self.owner_network_namespace(item) as namespace:
            gateway_identity = self.psql(IDENTITY, url=routed(gateway), namespace=namespace)
        admin_identity = self.psql(IDENTITY, database="quizcraft_v2")
        require(host_identity == gateway_identity == admin_identity, "QuizCraft owner route and host admin target differ")
        require(host_identity["database"] == "quizcraft_v2" and host_identity["port"] == 5432 and 160000 <= host_identity["version"] < 170000, "QuizCraft requires installed PostgreSQL 16 target")
        for name in ("psql", "pg_dump", "pg_restore", "createdb", "dropdb"):
            require(re.search(rb"\b16\.[0-9]+\b", self.run(name, "--version")), "PostgreSQL tool major mismatch")
        self.database_identity = host_identity

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
        data.update(scope=list(SERVICES), engine_sha256=engine.digest(BASE), candidate_compose_sha256=engine.digest(self.runtime / "docker-compose.henukit.release.yml"), quizcraft_migrations=after, database_identity=self.database_identity)
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
