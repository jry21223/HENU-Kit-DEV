"""Existing native PostgreSQL16 authority; no publishing or credential path."""

import contextlib
import fcntl
import ipaddress
import json
import os
from pathlib import Path
import re
import stat
import subprocess
from urllib.parse import parse_qsl, unquote, urlsplit, urlunsplit

IDENTITY = """SELECT json_build_object('database',current_database(),
'oid',(SELECT oid FROM pg_database WHERE datname=current_database()),
'started',to_char(pg_postmaster_start_time() AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'port',current_setting('port')::int,
'version',current_setting('server_version_num')::int)"""
NS_GET_NSTYPE = 0xB703
CLONE_NEWNET = 0x40000000


class QuizcraftDatabase:
    def init_quizcraft_tools(self):
        for name in (
            "psql",
            "pg_dump",
            "pg_restore",
            "createdb",
            "dropdb",
            "runuser",
            "nsenter",
        ):
            path = (
                self.prefix / "bin" / name
                if self.testing
                else Path("/usr/sbin/runuser")
                if name == "runuser"
                else Path("/usr/bin/nsenter")
                if name == "nsenter"
                else Path("/usr/lib/postgresql/16/bin") / name
            )
            self.trusted(path)
            self.tools[name] = str(path)

    def pg_environment(self, url=None):
        env = {k: v for k, v in self.env.items() if not k.startswith("PG")}
        env["PGCONNECT_TIMEOUT"] = "10"
        if url:
            target = urlsplit(url)
            env.update(
                PGHOST=target.hostname,
                PGPORT=str(target.port or 5432),
                PGDATABASE=unquote(target.path.removeprefix("/")),
                PGUSER=unquote(target.username or ""),
                PGPASSWORD=unquote(target.password or ""),
            )
            # The route gate permits only sslmode=disable (or no query).
            # Keep it in libpq's environment without a secret-bearing argv.
            for key, value in parse_qsl(target.query):
                self.require(
                    key == "sslmode" and value == "disable",
                    "unreviewed PostgreSQL query option",
                )
                env["PGSSLMODE"] = value
        return env

    def psql(self, query, *, url=None, database=None, namespace=None):
        args = [
            "--no-psqlrc",
            "--no-password",
            "--set",
            "ON_ERROR_STOP=1",
            "--tuples-only",
            "--no-align",
        ]
        if database:
            output = self.run(
                "runuser",
                "-u",
                "postgres",
                "--",
                self.tools["psql"],
                "--dbname",
                database,
                *args,
                "--command",
                query,
                env=self.pg_environment(),
            )
        elif namespace is None:
            output = self.run(
                "psql", *args, "--command", query, env=self.pg_environment(url)
            )
        else:
            # Pin the already-open network namespace across exec. No mount,
            # user or PID namespace is entered; the trusted host PG16 tool and
            # exact owner credentials remain the same.
            result = subprocess.run(
                [
                    self.tools["nsenter"],
                    "--net=/proc/self/fd/" + str(namespace),
                    "--",
                    self.tools["psql"],
                    *args,
                    "--command",
                    query,
                ],
                env=self.pg_environment(url),
                pass_fds=(namespace,),
                capture_output=True,
                timeout=60,
            )
            self.require(
                result.returncode == 0,
                "QuizCraft container-network psql command failed ("
                + str(result.returncode)
                + ")",
            )
            output = result.stdout
        return json.loads(output)

    def namespace_container_identity(self, item):
        state = item.get("State", {})
        pid = state.get("Pid")
        self.require(
            re.fullmatch(r"[0-9a-f]{64}", item.get("Id", ""))
            and item.get("Name") == "/henukit-quizcraft-1"
            and state.get("Running") is True
            and not state.get("Paused", False)
            and not state.get("Restarting", False)
            and type(pid) is int
            and pid > 1
            and state.get("StartedAt"),
            "QuizCraft container is not a stable running namespace owner",
        )
        return {
            "id": item["Id"],
            "pid": pid,
            "started": state["StartedAt"],
            "restarts": item["RestartCount"],
            "hosts": item["HostsPath"],
            "network": item["NetworkSettings"],
            "extra_hosts": item["HostConfig"].get("ExtraHosts"),
            "database_url": self.container_environment(item).get("DATABASE_URL"),
        }

    @contextlib.contextmanager
    def owner_network_namespace(self, item):
        expected = self.namespace_container_identity(item)
        namespace_path = self.path("/proc/" + str(expected["pid"]) + "/ns/net")
        sandbox = Path(item["NetworkSettings"].get("SandboxKey", ""))
        self.require(
            sandbox.parent
            in {self.path("/run/docker/netns"), self.path("/var/run/docker/netns")}
            and not sandbox.is_symlink(),
            "unreviewed QuizCraft Docker sandbox path",
        )
        # /var/run may be the system's root-owned /run alias. Validate the
        # resolved retained mount and every parent before using its identity.
        sandbox = sandbox.resolve(strict=True)
        self.trusted(sandbox)
        if self.testing:
            self.trusted(namespace_path)
        else:
            self.require(
                namespace_path.is_symlink()
                and re.fullmatch(r"net:\[[0-9]+\]", os.readlink(namespace_path)),
                "QuizCraft proc entry is not a kernel network namespace",
            )
        descriptor = os.open(namespace_path, os.O_RDONLY | os.O_CLOEXEC)
        try:
            opened = os.fstat(descriptor)
            self.require(
                stat.S_ISREG(opened.st_mode),
                "QuizCraft network namespace is not a regular namespace handle",
            )
            if not self.testing:
                self.require(
                    fcntl.ioctl(descriptor, NS_GET_NSTYPE) == CLONE_NEWNET,
                    "QuizCraft handle is not a Linux network namespace",
                )

            def unchanged():
                actual = json.loads(self.run("docker", "inspect", item["Id"]))
                self.require(
                    len(actual) == 1
                    and self.namespace_container_identity(actual[0]) == expected,
                    "QuizCraft container changed during namespace binding",
                )
                for path in (namespace_path, sandbox):
                    current = path.stat()
                    self.require(
                        (current.st_dev, current.st_ino)
                        == (opened.st_dev, opened.st_ino),
                        "QuizCraft process and Docker sandbox namespaces differ or changed",
                    )

            unchanged()
            yield descriptor
            unchanged()
        finally:
            os.close(descriptor)

    def bind_quizcraft_database(self, item):
        raw = self.container_environment(item).get("DATABASE_URL", "")
        target = urlsplit(raw)
        self.require(
            target.scheme in ("postgres", "postgresql")
            and target.hostname == "host.docker.internal"
            and target.port in (None, 5432)
            and target.path == "/quizcraft_v2"
            and target.username
            and target.password
            and not target.fragment
            and target.query in ("", "sslmode=disable"),
            "QuizCraft target is not the reviewed host PostgreSQL database",
        )
        self.require(
            "host.docker.internal:host-gateway"
            in item.get("HostConfig", {}).get("ExtraHosts", []),
            "QuizCraft host route is not Docker host-gateway",
        )
        # Docker's host-gateway may resolve to its default bridge, rather than
        # this container's Compose network gateway. Read the kernel-managed
        # container hosts file which Docker inspect binds to this exact ID.
        self.require(
            re.fullmatch(r"[0-9a-f]{64}", item["Id"]),
            "invalid QuizCraft container identity",
        )
        hosts = self.path("/var/lib/docker/containers/" + item["Id"] + "/hosts")
        self.require(
            item.get("HostsPath") == str(hosts),
            "QuizCraft hosts path differs from container identity",
        )
        self.trusted(hosts)
        addresses = set()
        for line in hosts.read_text().splitlines():
            fields = line.partition("#")[0].split()
            if len(fields) > 1 and "host.docker.internal" in fields[1:]:
                addresses.add(fields[0])
        self.require(len(addresses) == 1, "ambiguous QuizCraft host-gateway resolution")
        gateway = next(iter(addresses))
        self.require(
            ipaddress.ip_address(gateway).version == 4
            and ipaddress.ip_address(gateway).is_private,
            "unreviewed QuizCraft gateway address",
        )
        # Preserve the exact scheme, credentials, port spelling, DB and query.
        authority = target.netloc
        self.require(
            authority.count("host.docker.internal") == 1, "ambiguous database authority"
        )

        def routed(host):
            return urlunsplit(
                (
                    target.scheme,
                    authority.replace("host.docker.internal", host),
                    target.path,
                    target.query,
                    "",
                )
            )

        self.database_url = routed("127.0.0.1")
        host_identity = self.psql(IDENTITY, url=self.database_url)
        with self.owner_network_namespace(item) as namespace:
            gateway_identity = self.psql(
                IDENTITY, url=routed(gateway), namespace=namespace
            )
        admin_identity = self.psql(IDENTITY, database="quizcraft_v2")
        self.require(
            host_identity == gateway_identity == admin_identity,
            "QuizCraft owner route and host admin target differ",
        )
        self.require(
            host_identity["database"] == "quizcraft_v2"
            and host_identity["port"] == 5432
            and 160000 <= host_identity["version"] < 170000,
            "QuizCraft requires installed PostgreSQL 16 target",
        )
        for name in ("psql", "pg_dump", "pg_restore", "createdb", "dropdb"):
            self.require(
                re.search(rb"\b16\.[0-9]+\b", self.run(name, "--version")),
                "PostgreSQL tool major mismatch",
            )
        self.database_identity = host_identity
