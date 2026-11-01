#!/usr/bin/env python3
"""Adopt the audited mixed stack as one exact-SHA, getWork-disabled release."""

import argparse
import base64
import contextlib
import copy
import fcntl
import importlib.util
import hashlib
import hmac
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import shutil
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
    parser.add_argument(
        "--acknowledge-getwork-disabled", action="store_true", required=True
    )
    mode = parser.add_mutually_exclusive_group(required=True)
    for name in ("preflight", "execute", "rollback"):
        mode.add_argument("--" + name, action="store_true")
    result = parser.parse_args()
    if not re.fullmatch(r"[0-9a-f]{40}", result.sha) or not re.fullmatch(
        r"[1-9][0-9]*", result.run_id
    ):
        parser.error("a full lowercase SHA and numeric Actions run ID are required")
    return result


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


class Release(engine.Release):
    services = SCOPE
    helper_file = Path(__file__)
    state_scope = "stack-transition"
    receipt_format = FORMAT

    def __init__(self, options):
        super().__init__(options)
        self.guard = portal.Release(options)

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

    def preflight(self):
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
        self.guard.predecessor(containers)
        chain = self.guard.predecessor_chain(record)
        anchor = current.get("membership_checkpoint")
        require(anchor, "Portal v3 membership checkpoint is missing")
        member = self.guard.membership_checkpoint(anchor)
        prior = chain[-1][1]
        self.trusted(self.envfile, private=True)
        application = self.application(prior)
        before, sources = self.mixed_configuration(
            baseline, baseline_sha, current, prior, member, application
        )
        after = self.render(
            self.runtime,
            self.o.sha,
            self.o.sha,
            self.timestamp(),
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
        return {
            "format": FORMAT,
            "scope": list(SCOPE),
            "sha": self.o.sha,
            "run_id": self.o.run_id,
            "attempt": attempt,
            "runtime": str(self.runtime),
            "helper_sha256": engine.digest(self.helper_file),
            "baseline": baseline_sha,
            "baseline_runtime": str(baseline),
            "deployed_at": self.timestamp(),
            "environment_sha256": engine.digest(self.envfile),
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
        }

    @staticmethod
    def timestamp():
        return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())

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
        deployed = prior["deployed_at"]
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
        installer = self.runtime / "materials-runtime/install.sh"
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
        text = installer.read_text()
        table = re.search(r"declare -a install_records=\(\n(.*?)\n\)", text, re.S)
        require(table, "materials installer has no fixed target table")
        records = []
        for line in table[1].splitlines():
            match = re.fullmatch(r'\s*"([^|"\s]+)\|(/[^|"\s]+)\|(0[0-7]{3})"', line)
            require(match, "unreviewed materials installer target table")
            source, target, mode = match.groups()
            require(
                not any(p in ("..", ".") for p in Path(target).parts)
                and target.startswith(
                    (
                        "/usr/local/bin/henukit-",
                        "/usr/local/libexec/henukit/",
                        "/etc/systemd/system/henukit-",
                    )
                ),
                "materials target is outside constrained replay scope",
            )
            current = self.path(target)
            old = baseline / "materials-runtime" / source
            new = self.runtime / "materials-runtime" / source
            for path in (current, old, new):
                self.trusted(path)
            require(
                engine.digest(current) == engine.digest(old),
                "installed materials runtime differs from full baseline",
            )
            records.append(
                {
                    "source": source,
                    "target": target,
                    "mode": int(mode, 8),
                    "before_mode": current.stat().st_mode & 0o777,
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
        temporary = path.parent / (".transition-alias-" + secrets.token_hex(12))
        temporary.symlink_to(target)
        os.replace(temporary, path)
        self.durable_directory(path.parent)

    def checkpoint(self, data, phase):
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
            require(
                engine.digest(path)
                == target["after_sha256" if candidate else "before_sha256"],
                "installed materials runtime identity differs",
            )
            require(
                path.stat().st_mode & 0o777
                == target["mode" if candidate else "before_mode"],
                "installed materials runtime mode differs",
            )
        require(
            engine.digest(self.path("/etc/henukit-deploy/materials-activate.env"))
            == data["materials"]["activation_config_sha256"],
            "materials activation config drift",
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
                and engine.digest(path) == targets[target]["before_sha256"],
                "materials installer record is not replayable",
            )
            copies.append(
                (self.path(target), path.read_bytes(), path.stat().st_mode & 0o777)
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
        for path in (
            "/api/v1/account/summary",
            "/api/v1/console/workshop/catalog",
            "/api/v1/quizcraft/workshop/catalog",
        ):
            self.http(path, (401, 403))
        self.http("/api/v1/bot/membership-entitlement", (401, 403))
        self.http("/quiz/", (404,))
        self.http("/study-api/healthz", (404,))
        if candidate:
            self.disabled_source(data)
        self.retained_unchanged(data)

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
        actual = self.runtime_inventory(self.runtime)
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
        for path in (
            self.path("/opt/henukit/current"),
            self.path("/opt/henukit-current"),
        ):
            self.alias(path, data["baseline_runtime"])
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
                for path in (
                    self.path("/opt/henukit/current"),
                    self.path("/opt/henukit-current"),
                ):
                    self.alias(path, self.runtime)
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
