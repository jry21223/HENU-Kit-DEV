#!/usr/bin/env python3
"""Fail-closed verification for the HENUKit WSL Job Source MCP node."""

from __future__ import annotations

import argparse
import errno
import hashlib
import ipaddress
import json
import os
import pathlib
import pwd
import re
import stat
import subprocess
import sys
import tarfile
import time
import urllib.request
from typing import BinaryIO, NamedTuple, Protocol


class VerificationError(RuntimeError):
    pass


PINNED_SOURCES = (
    "alibaba", "baidu", "beike", "bytedance", "ctrip", "dewu", "didi", "jd",
    "kuaishou", "meituan", "netease", "pdd", "tencent", "tencentmusic",
    "tongcheng", "vipshop", "xfusion", "xiaohongshu",
)
UNIT_NAMES = ("henukit-getwork-mcp.service", "henukit-getwork-tunnel.service")
REPOSITORY_URL = "https://github.com/jry21223/HENU-Kit-DEV.git"
ACTIONS_PREDICATE_TYPE = (
    "https://github.com/jry21223/HENU-Kit-DEV/attestations/getwork-actions-release-v1"
)


class Config(NamedTuple):
    release_sha: str
    token_file: pathlib.Path
    node_env_file: pathlib.Path
    private_key_file: pathlib.Path
    known_hosts_file: pathlib.Path
    artifact_file: pathlib.Path
    source_unit_dir: pathlib.Path
    installed_unit_dir: pathlib.Path
    installed_egress_file: pathlib.Path
    trust_file: pathlib.Path
    manifest_file: pathlib.Path
    signature_file: pathlib.Path
    allowed_signers_file: pathlib.Path
    provenance_mode: str = "ssh-signature"
    attestation_file: pathlib.Path | None = None
    gh_file: pathlib.Path = pathlib.Path("/usr/bin/gh")


class SecureFile(NamedTuple):
    regular: bool
    symlink: bool
    owner_uid: int
    mode: int
    contents: str


class Evidence(NamedTuple):
    image: str
    image_id: str
    platform: str
    archive_sha256: str
    source_count: int
    tools: tuple[str, ...]
    crawl_source: str


class Probe(Protocol):
    def current_main_sha(self) -> str: ...
    def osrelease(self) -> str: ...
    def machine(self) -> str: ...
    def root_fstype(self) -> str: ...
    def docker_platform(self, image: str) -> str: ...
    def docker_image_id(self, image: str) -> str: ...
    def trusted_parent_chain(self, path: pathlib.Path) -> bool: ...
    def account_uid(self, name: str) -> int: ...
    def account_contract(self, name: str) -> bool: ...
    def secure_file(self, path: pathlib.Path) -> SecureFile: ...
    def unit_matches_source(self, name: str, source_dir: pathlib.Path, installed_dir: pathlib.Path) -> bool: ...
    def private_key_fingerprint(self, path: pathlib.Path) -> str: ...
    def known_host_fingerprints(self, path: pathlib.Path, host: str, port: int) -> list[str]: ...
    def signed_manifest_valid(
        self, manifest: pathlib.Path, signature: pathlib.Path, allowed_signers: pathlib.Path
    ) -> bool: ...
    def actions_attestation_valid(
        self,
        manifest: pathlib.Path,
        attestation: pathlib.Path,
        gh_file: pathlib.Path,
        release_sha: str,
    ) -> bool: ...
    def archive_sha256(self, path: pathlib.Path) -> str: ...
    def archive_image_id(self, path: pathlib.Path) -> str: ...
    def runtime_hardened(self, expected_image_id: str) -> bool: ...
    def egress_policy_live(self) -> bool: ...
    def service_active(self, name: str) -> bool: ...
    def health(self) -> dict[str, object]: ...
    def tools(self, token: str) -> list[str]: ...
    def sources(self, token: str) -> list[str]: ...
    def crawl(self, token: str, source: str) -> dict[str, object]: ...


def _env(contents: str) -> dict[str, str]:
    values: dict[str, str] = {}
    for line in contents.splitlines():
        if not line or line.startswith("#"):
            continue
        key, separator, value = line.partition("=")
        if not separator or not re.fullmatch(r"[A-Z0-9_]+", key) or key in values:
            raise VerificationError("deployment environment file is malformed")
        values[key] = value
    return values


def _rpc_result(envelope: object, expected_id: int) -> dict[str, object]:
    if not isinstance(envelope, dict):
        raise VerificationError("MCP response is not a JSON object")
    identifier = envelope.get("id")
    if (
        envelope.get("jsonrpc") != "2.0"
        or type(identifier) is not int
        or identifier != expected_id
        or "error" in envelope
    ):
        raise VerificationError("MCP response envelope is invalid")
    result = envelope.get("result")
    if not isinstance(result, dict):
        raise VerificationError("MCP response result is invalid")
    return result


def _tool_payload(result: dict[str, object], name: str) -> dict[str, object]:
    if "isError" in result:
        is_error = result["isError"]
        if type(is_error) is not bool or is_error is True:
            raise VerificationError(f"MCP tool {name} returned invalid isError")
    content_items = result.get("content")
    if not isinstance(content_items, list):
        raise VerificationError(f"MCP tool {name} content is invalid")
    for content in content_items:
        if not isinstance(content, dict) or not isinstance(content.get("type"), str):
            raise VerificationError(f"MCP tool {name} content entry is invalid")
        if content["type"] != "text":
            continue
        text = content.get("text")
        if not isinstance(text, str):
            raise VerificationError(f"MCP tool {name} text content is invalid")
        decoded = json.loads(text)
        if not isinstance(decoded, dict):
            raise VerificationError(f"MCP tool {name} payload is invalid")
        return decoded
    raise VerificationError(f"MCP tool {name} returned no text content")


def _read_bounded_json(stream: BinaryIO, max_bytes: int, deadline: float) -> dict[str, object]:
    chunks: list[bytes] = []
    size = 0
    while True:
        if time.monotonic() > deadline:
            raise VerificationError("MCP response exceeded its total deadline")
        chunk = stream.read(min(65536, max_bytes + 1 - size))
        if not chunk:
            break
        if not isinstance(chunk, bytes):
            raise VerificationError("MCP response stream did not return bytes")
        chunks.append(chunk)
        size += len(chunk)
        if size > max_bytes:
            raise VerificationError("MCP response exceeded its size limit")
    decoded = json.loads(b"".join(chunks))
    if not isinstance(decoded, dict):
        raise VerificationError("MCP response is not a JSON object")
    return decoded


def _require_file(
    probe: Probe, path: pathlib.Path, mode: int, label: str, owner_uid: int = 0
) -> SecureFile:
    if not path.is_absolute() or not probe.trusted_parent_chain(path):
        raise VerificationError(f"{label} parent chain is not trusted")
    item = probe.secure_file(path)
    if not item.regular or item.symlink or item.owner_uid != owner_uid or item.mode != mode:
        raise VerificationError(f"{label} metadata is invalid")
    return item


def verify(config: Config, probe: Probe) -> Evidence:
    if not re.fullmatch(r"[0-9a-f]{40}", config.release_sha):
        raise VerificationError("release SHA must be 40 lowercase hexadecimal characters")
    if probe.current_main_sha() != config.release_sha:
        raise VerificationError("release SHA is not the freshly fetched current origin/main")
    if "microsoft" not in probe.osrelease().lower():
        raise VerificationError("Job Source MCP node must run on WSL2")
    if probe.machine() != "x86_64" or probe.root_fstype() != "ext4":
        raise VerificationError("Job Source MCP node must use Linux x86_64 on ext4")
    _require_file(
        probe, pathlib.Path(__file__).resolve(), 0o755, "installed node verifier"
    )

    node_env = _env(_require_file(probe, config.node_env_file, 0o644, "node env").contents)
    required = {
        "HENUKIT_GETWORK_RELEASE_SHA", "HENUKIT_GETWORK_IMAGE_ID",
        "HENUKIT_GETWORK_ARCHIVE_SHA256", "HENUKIT_GETWORK_MEMORY_LIMIT",
        "HENUKIT_GETWORK_TUNNEL_TARGET", "HENUKIT_GETWORK_TUNNEL_PORT",
        "HENUKIT_GETWORK_MCP_UNIT_SHA256", "HENUKIT_GETWORK_TUNNEL_UNIT_SHA256",
        "HENUKIT_GETWORK_EGRESS_SHA256",
        "HENUKIT_GETWORK_PROVENANCE_MODE",
    }
    if set(node_env) != required:
        raise VerificationError("node env keys do not match the reviewed contract")
    if node_env["HENUKIT_GETWORK_RELEASE_SHA"] != config.release_sha:
        raise VerificationError("node env release SHA does not match")
    if (
        config.provenance_mode not in {"ssh-signature", "github-actions"}
        or node_env["HENUKIT_GETWORK_PROVENANCE_MODE"] != config.provenance_mode
    ):
        raise VerificationError("node env provenance mode does not match")
    expected_image_id = node_env["HENUKIT_GETWORK_IMAGE_ID"]
    expected_archive_sha = node_env["HENUKIT_GETWORK_ARCHIVE_SHA256"]
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", expected_image_id):
        raise VerificationError("node env image ID is invalid")
    if not re.fullmatch(r"[0-9a-f]{64}", expected_archive_sha):
        raise VerificationError("node env archive checksum is invalid")
    if node_env["HENUKIT_GETWORK_MEMORY_LIMIT"] != "4g":
        raise VerificationError("node env memory limit does not match the reviewed value")
    target_match = re.fullmatch(
        r"henukit-getwork-tunnel@([A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?)",
        node_env["HENUKIT_GETWORK_TUNNEL_TARGET"],
    )
    try:
        tunnel_port = int(node_env["HENUKIT_GETWORK_TUNNEL_PORT"])
    except ValueError as error:
        raise VerificationError("node env tunnel port is invalid") from error
    if target_match is None or not 1 <= tunnel_port <= 65535:
        raise VerificationError("node env tunnel target is invalid")

    secret = _env(_require_file(probe, config.token_file, 0o600, "MCP secret").contents)
    if set(secret) != {"GETWORK_MCP_ACCESS_TOKEN"}:
        raise VerificationError("MCP secret keys do not match the reviewed contract")
    token = secret["GETWORK_MCP_ACCESS_TOKEN"].strip()
    lowered_token = token.lower()
    if (
        len(token) < 32 or any(character.isspace() for character in token)
        or any(marker in lowered_token for marker in ("replace", "example", "change-me", "test-only"))
    ):
        raise VerificationError("getWork token is invalid")

    probe.account_uid("henukit-getwork-tunnel")
    if not probe.account_contract("henukit-getwork-tunnel"):
        raise VerificationError("tunnel account does not match the no-login contract")
    trust = _env(_require_file(probe, config.trust_file, 0o600, "fingerprint trust").contents)
    if set(trust) != {
        "HENUKIT_GETWORK_TUNNEL_KEY_FINGERPRINT",
        "HENUKIT_GETWORK_HOST_KEY_FINGERPRINT",
    }:
        raise VerificationError("fingerprint trust keys do not match the reviewed contract")
    _require_file(probe, config.private_key_file, 0o600, "tunnel private key")
    if (
        probe.private_key_fingerprint(config.private_key_file)
        != trust["HENUKIT_GETWORK_TUNNEL_KEY_FINGERPRINT"]
    ):
        raise VerificationError("tunnel private key fingerprint is not approved")
    _require_file(probe, config.known_hosts_file, 0o640, "known hosts")
    host_fingerprints = probe.known_host_fingerprints(
        config.known_hosts_file, target_match.group(1), tunnel_port
    )
    if host_fingerprints != [trust["HENUKIT_GETWORK_HOST_KEY_FINGERPRINT"]]:
        raise VerificationError("production host key fingerprint is not approved")

    for unit in UNIT_NAMES:
        _require_file(probe, config.source_unit_dir / unit, 0o644, f"source {unit}")
        installed_unit = _require_file(
            probe, config.installed_unit_dir / unit, 0o644, unit
        )
        digest_key = (
            "HENUKIT_GETWORK_MCP_UNIT_SHA256"
            if unit == "henukit-getwork-mcp.service"
            else "HENUKIT_GETWORK_TUNNEL_UNIT_SHA256"
        )
        if hashlib.sha256(installed_unit.contents.encode()).hexdigest() != node_env[digest_key]:
            raise VerificationError(f"{unit} checksum differs from the installed contract")
        if not probe.unit_matches_source(unit, config.source_unit_dir, config.installed_unit_dir):
            raise VerificationError(f"{unit} differs from the exact source unit")
        if not probe.service_active(unit):
            raise VerificationError(f"{unit} is not active")

    installed_egress = _require_file(
        probe, config.installed_egress_file, 0o755, "egress policy helper"
    )
    source_egress = _require_file(
        probe,
        config.source_unit_dir.parent / "henukit-getwork-egress",
        0o755,
        "source egress policy helper",
    )
    if (
        not source_egress.regular
        or source_egress.symlink
        or installed_egress.contents != source_egress.contents
        or hashlib.sha256(installed_egress.contents.encode()).hexdigest()
        != node_env["HENUKIT_GETWORK_EGRESS_SHA256"]
    ):
        raise VerificationError("egress policy helper differs from the exact installed contract")
    if not probe.egress_policy_live():
        raise VerificationError("live crawler egress policy is not hardened")

    manifest = _require_file(probe, config.manifest_file, 0o400, "release manifest")
    if config.provenance_mode == "ssh-signature":
        _require_file(probe, config.signature_file, 0o400, "release manifest signature")
        _require_file(probe, config.allowed_signers_file, 0o644, "release allowed signers")
        if not probe.signed_manifest_valid(
            config.manifest_file, config.signature_file, config.allowed_signers_file
        ):
            raise VerificationError("release manifest signature is invalid")
    else:
        if config.attestation_file is None:
            raise VerificationError("GitHub Actions attestation path is missing")
        _require_file(probe, config.attestation_file, 0o400, "Actions attestation")
        _require_file(probe, config.gh_file, 0o755, "GitHub CLI")
        if not probe.actions_attestation_valid(
            config.manifest_file,
            config.attestation_file,
            config.gh_file,
            config.release_sha,
        ):
            raise VerificationError("GitHub Actions attestation is invalid")
    _require_file(probe, config.artifact_file, 0o400, "getWork image archive")
    archive_sha = probe.archive_sha256(config.artifact_file)
    if archive_sha != expected_archive_sha:
        raise VerificationError("getWork image archive checksum does not match")
    archive_record = (
        f"artifact_sha256={archive_sha}  {config.artifact_file.name}"
    )
    manifest_lines = manifest.contents.splitlines()
    common_manifest_lines = (
        manifest_lines.count(f"release_sha={config.release_sha}") == 1
        and manifest_lines.count("source_ref=refs/heads/main") == 1
        and manifest_lines.count("builder_platform=linux/amd64") == 1
        and manifest_lines.count(archive_record) == 1
    )
    if config.provenance_mode == "ssh-signature":
        provenance_lines = (
            manifest_lines.count("format=henukit-local-release-v1") == 1
            and manifest_lines.count("signer=henukit-release") == 1
            and manifest_lines.count("signature_namespace=henukit-release") == 1
        )
    else:
        provenance_lines = (
            manifest_lines.count("format=henukit-getwork-actions-release-v1") == 1
            and manifest_lines.count("source_repository=jry21223/HENU-Kit-DEV") == 1
            and manifest_lines.count(
                "signer_workflow=.github/workflows/deploy-henukit.yml"
            )
            == 1
        )
    if not common_manifest_lines or not provenance_lines:
        raise VerificationError("release manifest does not bind the getWork archive")

    image = f"henukit-getwork-mcp:{config.release_sha}"
    platform = probe.docker_platform(image)
    image_id = probe.docker_image_id(image)
    archive_image_id = probe.archive_image_id(config.artifact_file)
    if (
        platform != "linux/amd64"
        or image_id != expected_image_id
        or image_id != archive_image_id
    ):
        raise VerificationError("getWork image identity or platform does not match")
    if not probe.runtime_hardened(image_id):
        raise VerificationError(
            "live crawler runtime is not the provenance-verified hardened image"
        )

    health = probe.health()
    if health.get("ok") is not True or health.get("upstream") != "RyaoVen/getWork@2c7800d":
        raise VerificationError("getWork health identity is invalid")
    tools = tuple(sorted(probe.tools(token)))
    if tools != ("crawl_jobs", "list_sources"):
        raise VerificationError("getWork MCP tool surface is not read-only and exact")
    sources = probe.sources(token)
    if tuple(sorted(sources)) != PINNED_SOURCES:
        raise VerificationError("getWork MCP source set does not match the pinned source set")
    crawl_source = PINNED_SOURCES[0]
    crawl = probe.crawl(token, crawl_source)
    if crawl.get("status") != "ok" or crawl.get("source") not in (None, crawl_source):
        raise VerificationError("real crawl_jobs preflight failed")
    if probe.current_main_sha() != config.release_sha:
        raise VerificationError("origin/main changed during node verification")

    return Evidence(image, image_id, platform, archive_sha, len(sources), tools, crawl_source)


class RealProbe:
    endpoint = "http://127.0.0.1:8100"

    @staticmethod
    def _command(*arguments: str) -> str:
        result = subprocess.run(arguments, check=True, capture_output=True, text=True)
        return result.stdout.strip()

    def osrelease(self) -> str:
        return self._command("uname", "-r")

    def current_main_sha(self) -> str:
        environment = {
            "PATH": "/usr/bin:/bin",
            "HOME": "/var/empty",
            "XDG_CONFIG_HOME": "/var/empty",
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_CONFIG_SYSTEM": "/dev/null",
            "GIT_CONFIG_GLOBAL": "/dev/null",
            "GIT_TERMINAL_PROMPT": "0",
        }
        result = subprocess.run(
            (
                "/usr/bin/git",
                "-c",
                "credential.helper=",
                "-c",
                "protocol.allow=never",
                "-c",
                "protocol.https.allow=always",
                "ls-remote",
                "--exit-code",
                REPOSITORY_URL,
                "refs/heads/main",
            ),
            check=True,
            capture_output=True,
            text=True,
            env=environment,
            cwd="/",
            timeout=60,
        )
        return result.stdout.split(maxsplit=1)[0]

    def machine(self) -> str:
        return self._command("uname", "-m")

    def root_fstype(self) -> str:
        return self._command("findmnt", "-no", "FSTYPE", "/")

    def docker_platform(self, image: str) -> str:
        return self._command("docker", "image", "inspect", image, "--format", "{{.Os}}/{{.Architecture}}")

    def docker_image_id(self, image: str) -> str:
        return self._command("docker", "image", "inspect", image, "--format", "{{.Id}}")

    def trusted_parent_chain(self, path: pathlib.Path) -> bool:
        current = path.parent
        while True:
            metadata = current.lstat()
            if not stat.S_ISDIR(metadata.st_mode) or stat.S_ISLNK(metadata.st_mode):
                return False
            if metadata.st_uid != 0 or stat.S_IMODE(metadata.st_mode) & 0o022:
                return False
            if current == current.parent:
                return True
            current = current.parent

    def account_uid(self, name: str) -> int:
        return pwd.getpwnam(name).pw_uid

    def account_contract(self, name: str) -> bool:
        passwd_fields = self._command("getent", "passwd", name).split(":")
        shadow_fields = self._command("getent", "shadow", name).split(":")
        group_fields = self._command("getent", "group", name).split(":")
        if len(passwd_fields) != 7 or len(shadow_fields) < 2 or len(group_fields) < 4:
            return False
        home = pathlib.Path(passwd_fields[5])
        metadata = home.lstat()
        return (
            int(passwd_fields[2]) < 1000
            and int(passwd_fields[3]) < 1000
            and passwd_fields[3] == group_fields[2]
            and passwd_fields[5] == "/var/lib/henukit-getwork-tunnel"
            and passwd_fields[6] == "/usr/sbin/nologin"
            and shadow_fields[1] == "NP"
            and group_fields[3] == ""
            and self._command("id", "-G", name) == passwd_fields[3]
            and stat.S_ISDIR(metadata.st_mode)
            and not stat.S_ISLNK(metadata.st_mode)
            and metadata.st_uid == 0
            and stat.S_IMODE(metadata.st_mode) == 0o750
        )

    def secure_file(self, path: pathlib.Path) -> SecureFile:
        metadata = path.lstat()
        if stat.S_ISLNK(metadata.st_mode):
            return SecureFile(False, True, metadata.st_uid, stat.S_IMODE(metadata.st_mode), "")
        flags = os.O_RDONLY | getattr(os, "O_CLOEXEC", 0) | getattr(os, "O_NOFOLLOW", 0)
        try:
            descriptor = os.open(path, flags)
        except OSError as error:
            if error.errno == errno.ELOOP:
                return SecureFile(False, True, metadata.st_uid, stat.S_IMODE(metadata.st_mode), "")
            raise
        with os.fdopen(descriptor, "rb") as stream:
            opened = os.fstat(stream.fileno())
            contents = (
                b""
                if path.name.endswith(".docker.tar.gz")
                else stream.read(1024 * 1024 + 1)
            )
        if len(contents) > 1024 * 1024:
            contents = b""
        return SecureFile(
            stat.S_ISREG(opened.st_mode), False, opened.st_uid,
            stat.S_IMODE(opened.st_mode), contents.decode("utf-8", errors="strict"),
        )

    def unit_matches_source(self, name: str, source_dir: pathlib.Path, installed_dir: pathlib.Path) -> bool:
        expected = self.secure_file(source_dir / name)
        installed = self.secure_file(installed_dir / name)
        return expected.regular and not expected.symlink and expected.contents == installed.contents

    def private_key_fingerprint(self, path: pathlib.Path) -> str:
        public_key = self._command("ssh-keygen", "-y", "-f", str(path))
        result = subprocess.run(
            ("ssh-keygen", "-lf", "-", "-E", "sha256"),
            input=public_key + "\n",
            check=True,
            capture_output=True,
            text=True,
        )
        fields = result.stdout.split()
        return fields[1] if len(fields) >= 2 else ""

    def known_host_fingerprints(self, path: pathlib.Path, host: str, port: int) -> list[str]:
        found = subprocess.run(
            ("ssh-keygen", "-F", f"[{host}]:{port}", "-f", str(path)),
            check=False,
            capture_output=True,
            text=True,
        )
        if found.returncode != 0 or not found.stdout.strip():
            return []
        result = subprocess.run(
            ("ssh-keygen", "-lf", "-", "-E", "sha256"),
            input=found.stdout,
            check=True,
            capture_output=True,
            text=True,
        )
        return sorted({line.split()[1] for line in result.stdout.splitlines() if len(line.split()) >= 2})

    def signed_manifest_valid(
        self, manifest: pathlib.Path, signature: pathlib.Path, allowed_signers: pathlib.Path
    ) -> bool:
        result = subprocess.run(
            (
                "ssh-keygen", "-Y", "verify", "-f", str(allowed_signers),
                "-I", "henukit-release", "-n", "henukit-release", "-s", str(signature),
            ),
            input=self.secure_file(manifest).contents,
            check=False,
            capture_output=True,
            text=True,
        )
        return result.returncode == 0

    def actions_attestation_valid(
        self,
        manifest: pathlib.Path,
        attestation: pathlib.Path,
        gh_file: pathlib.Path,
        release_sha: str,
    ) -> bool:
        environment = dict(os.environ)
        environment.pop("GH_TOKEN", None)
        environment.pop("GITHUB_TOKEN", None)
        environment.update({"GH_PROMPT_DISABLED": "1", "NO_COLOR": "1"})
        result = subprocess.run(
            (
                str(gh_file),
                "attestation",
                "verify",
                str(manifest),
                "--repo",
                "jry21223/HENU-Kit-DEV",
                "--bundle",
                str(attestation),
                "--signer-workflow",
                "jry21223/HENU-Kit-DEV/.github/workflows/deploy-henukit.yml",
                "--source-ref",
                "refs/heads/main",
                "--source-digest",
                release_sha,
                "--predicate-type",
                ACTIONS_PREDICATE_TYPE,
                "--deny-self-hosted-runners",
                "--format",
                "json",
            ),
            check=False,
            capture_output=True,
            text=True,
            env=environment,
            timeout=60,
        )
        if result.returncode != 0:
            return False
        decoded = json.loads(result.stdout)
        return isinstance(decoded, list) and len(decoded) == 1

    def archive_sha256(self, path: pathlib.Path) -> str:
        digest = hashlib.sha256()
        flags = os.O_RDONLY | getattr(os, "O_CLOEXEC", 0) | getattr(os, "O_NOFOLLOW", 0)
        descriptor = os.open(path, flags)
        with os.fdopen(descriptor, "rb") as stream:
            while chunk := stream.read(1024 * 1024):
                digest.update(chunk)
        return digest.hexdigest()

    def archive_image_id(self, path: pathlib.Path) -> str:
        with tarfile.open(path, mode="r:gz") as archive:
            member = archive.getmember("manifest.json")
            if not member.isfile() or member.size > 1024 * 1024:
                return ""
            extracted = archive.extractfile(member)
            if extracted is None:
                return ""
            decoded = json.load(extracted)
        if not isinstance(decoded, list) or len(decoded) != 1 or not isinstance(decoded[0], dict):
            return ""
        config_name = decoded[0].get("Config")
        if not isinstance(config_name, str) or re.fullmatch(r"[0-9a-f]{64}\.json", config_name) is None:
            return ""
        return "sha256:" + config_name.removesuffix(".json")

    def runtime_hardened(self, expected_image_id: str) -> bool:
        decoded = json.loads(self._command("docker", "inspect", "henukit-getwork-mcp"))
        if not isinstance(decoded, list) or len(decoded) != 1 or not isinstance(decoded[0], dict):
            return False
        container = decoded[0]
        config = container.get("Config")
        host = container.get("HostConfig")
        state = container.get("State")
        network_settings = container.get("NetworkSettings")
        if (
            not isinstance(config, dict)
            or not isinstance(host, dict)
            or not isinstance(state, dict)
            or not isinstance(network_settings, dict)
        ):
            return False
        networks = network_settings.get("Networks")
        if not isinstance(networks, dict) or set(networks) != {"henukit-getwork-egress"}:
            return False
        egress_network = networks["henukit-getwork-egress"]
        if not isinstance(egress_network, dict):
            return False
        try:
            address = ipaddress.ip_address(str(egress_network.get("IPAddress", "")))
        except ValueError:
            return False
        bindings = host.get("PortBindings")
        port = bindings.get("8100/tcp") if isinstance(bindings, dict) else None
        return (
            container.get("Image") == expected_image_id
            and state.get("Running") is True
            and address in ipaddress.ip_network("172.30.250.0/24")
            and config.get("User") == "65532:65532"
            and host.get("ReadonlyRootfs") is True
            and host.get("NetworkMode") == "henukit-getwork-egress"
            and host.get("Privileged") is False
            and isinstance(host.get("CapDrop"), list)
            and sorted(str(value).upper() for value in host["CapDrop"]) == ["ALL"]
            and isinstance(host.get("SecurityOpt"), list)
            and any(str(value).startswith("no-new-privileges") for value in host["SecurityOpt"])
            and isinstance(port, list)
            and len(port) == 1
            and port[0] == {"HostIp": "127.0.0.1", "HostPort": "8100"}
        )

    def egress_policy_live(self) -> bool:
        decoded = json.loads(self._command("docker", "network", "inspect", "henukit-getwork-egress"))
        if not isinstance(decoded, list) or len(decoded) != 1 or not isinstance(decoded[0], dict):
            return False
        network = decoded[0]
        ipam = network.get("IPAM")
        configs = ipam.get("Config") if isinstance(ipam, dict) else None
        options = network.get("Options")
        if (
            network.get("EnableIPv6") is not False
            or not isinstance(configs, list)
            or len(configs) != 1
            or configs[0].get("Subnet") != "172.30.250.0/24"
            or not isinstance(options, dict)
            or options.get("com.docker.network.bridge.name") != "henukit-getwork0"
        ):
            return False
        docker_user = self._command("iptables", "-S", "DOCKER-USER").splitlines()
        jump = "-A DOCKER-USER -s 172.30.250.0/24 -j HENUKIT-GETWORK-EGRESS"
        active_rules = [line for line in docker_user if line.startswith("-A ")]
        if not active_rules or active_rules[0] != jump:
            return False
        actual = [
            line for line in self._command("iptables", "-S", "HENUKIT-GETWORK-EGRESS").splitlines()
            if line.startswith("-A ")
        ]
        destinations = (
            "0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
            "169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16",
            "224.0.0.0/4", "240.0.0.0/4",
        )
        if len(actual) != len(destinations) + 2:
            return False
        if actual[0] != "-A HENUKIT-GETWORK-EGRESS -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT":
            return False
        for line, destination in zip(actual[1:-1], destinations, strict=True):
            prefix = f"-A HENUKIT-GETWORK-EGRESS -d {destination} -j REJECT"
            if line not in (prefix, prefix + " --reject-with icmp-port-unreachable"):
                return False
        return actual[-1] == "-A HENUKIT-GETWORK-EGRESS -j RETURN"

    def service_active(self, name: str) -> bool:
        return self._command("systemctl", "is-active", name) == "active"

    def _json_request(
        self,
        path: str,
        payload: dict[str, object] | None = None,
        token: str = "",
        *,
        max_bytes: int,
        total_seconds: float,
    ) -> dict[str, object]:
        data = None if payload is None else json.dumps(payload, separators=(",", ":")).encode()
        request = urllib.request.Request(self.endpoint + path, data=data)
        request.add_header("Accept", "application/json, text/event-stream")
        if payload is not None:
            request.add_header("Content-Type", "application/json")
        if token:
            request.add_header("Authorization", "Bearer " + token)
        deadline = time.monotonic() + total_seconds
        with urllib.request.urlopen(request, timeout=min(30.0, total_seconds)) as response:
            return _read_bounded_json(response, max_bytes, deadline)

    def _call(self, token: str, request_id: int, name: str, arguments: dict[str, object]) -> dict[str, object]:
        envelope = self._json_request(
            "/mcp",
            {"jsonrpc": "2.0", "id": request_id, "method": "tools/call", "params": {"name": name, "arguments": arguments}},
            token,
            max_bytes=16 * 1024 * 1024,
            total_seconds=420 if name == "crawl_jobs" else 10,
        )
        result = _rpc_result(envelope, request_id)
        return _tool_payload(result, name)

    def health(self) -> dict[str, object]:
        return self._json_request("/healthz", max_bytes=16384, total_seconds=5)

    def tools(self, token: str) -> list[str]:
        envelope = self._json_request(
            "/mcp",
            {"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}},
            token,
            max_bytes=1024 * 1024,
            total_seconds=10,
        )
        result = _rpc_result(envelope, 1)
        tools = result.get("tools")
        if not isinstance(tools, list):
            raise VerificationError("MCP tools/list result is invalid")
        names: list[str] = []
        for tool in tools:
            if not isinstance(tool, dict) or not isinstance(tool.get("name"), str):
                raise VerificationError("MCP tools/list entry is invalid")
            names.append(tool["name"])
        return names

    def sources(self, token: str) -> list[str]:
        decoded = self._call(token, 2, "list_sources", {})
        sources = decoded.get("sources")
        if not isinstance(sources, list):
            raise VerificationError("list_sources payload is invalid")
        keys: list[str] = []
        for source in sources:
            if not isinstance(source, dict) or not isinstance(source.get("key"), str):
                raise VerificationError("list_sources entry is invalid")
            keys.append(source["key"])
        return keys

    def crawl(self, token: str, source: str) -> dict[str, object]:
        return self._call(token, 3, "crawl_jobs", {"source": source, "since_days": 1})


def main(arguments: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Verify the HENUKit WSL Job Source MCP node")
    parser.add_argument("--sha", required=True)
    parser.add_argument("--token-file", type=pathlib.Path, required=True)
    parser.add_argument("--artifact-file", type=pathlib.Path, required=True)
    parser.add_argument("--node-env-file", type=pathlib.Path, default=pathlib.Path("/etc/henukit-getwork/node.env"))
    parser.add_argument("--private-key-file", type=pathlib.Path, default=pathlib.Path("/etc/henukit-getwork/tunnel/id_ed25519"))
    parser.add_argument("--known-hosts-file", type=pathlib.Path, default=pathlib.Path("/etc/henukit-getwork/tunnel/known_hosts"))
    parser.add_argument("--installed-unit-dir", type=pathlib.Path, default=pathlib.Path("/etc/systemd/system"))
    parser.add_argument("--installed-egress-file", type=pathlib.Path, default=pathlib.Path("/usr/local/libexec/henukit-getwork-egress"))
    parser.add_argument("--trust-file", type=pathlib.Path, default=pathlib.Path("/etc/henukit-getwork/trust.env"))
    parser.add_argument("--allowed-signers-file", type=pathlib.Path, default=pathlib.Path("/etc/henukit-getwork/release-signers"))
    parser.add_argument(
        "--provenance-mode",
        choices=("ssh-signature", "github-actions"),
        default="ssh-signature",
    )
    parser.add_argument("--actions-attestation-file", type=pathlib.Path)
    parser.add_argument("--gh-file", type=pathlib.Path, default=pathlib.Path("/usr/bin/gh"))
    parser.add_argument("--manifest-file", type=pathlib.Path)
    parser.add_argument("--signature-file", type=pathlib.Path)
    options = parser.parse_args(arguments)
    source_units = pathlib.Path(__file__).resolve().parent / "systemd"
    default_manifest = (
        f"henukit-getwork-actions-{options.sha}.manifest"
        if options.provenance_mode == "github-actions"
        else f"henukit-release-{options.sha}.manifest"
    )
    manifest_file = options.manifest_file or options.artifact_file.parent / default_manifest
    signature_file = options.signature_file or manifest_file.with_name(manifest_file.name + ".sig")
    attestation_file = options.actions_attestation_file
    if options.provenance_mode == "github-actions" and attestation_file is None:
        attestation_file = options.artifact_file.parent / f"henukit-getwork-actions-{options.sha}.attestation.json"
    try:
        evidence = verify(
            Config(
                release_sha=options.sha,
                token_file=options.token_file,
                node_env_file=options.node_env_file,
                private_key_file=options.private_key_file,
                known_hosts_file=options.known_hosts_file,
                artifact_file=options.artifact_file,
                source_unit_dir=source_units,
                installed_unit_dir=options.installed_unit_dir,
                installed_egress_file=options.installed_egress_file,
                trust_file=options.trust_file,
                manifest_file=manifest_file,
                signature_file=signature_file,
                allowed_signers_file=options.allowed_signers_file,
                provenance_mode=options.provenance_mode,
                attestation_file=attestation_file,
                gh_file=options.gh_file,
            ),
            RealProbe(),
        )
    except (VerificationError, OSError, subprocess.SubprocessError, ValueError, UnicodeDecodeError, json.JSONDecodeError) as error:
        print(f"verification failed: {error}", file=sys.stderr)
        return 1
    print(json.dumps(evidence._asdict(), sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
