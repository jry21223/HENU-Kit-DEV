#!/usr/bin/env python3
"""Create a one-request curl config for QuizCraft's signed Portal-read API."""

from __future__ import annotations

import argparse
import base64
import hashlib
import hmac
import os
import pathlib
import re
import secrets
import shlex
import sys
import tempfile
import time
import urllib.parse


REQUIRED_ENVIRONMENT_KEYS = (
    "QUIZCRAFT_PORTAL_CATALOG_CLIENT_ID",
    "QUIZCRAFT_PORTAL_CATALOG_KEY_ID",
    "QUIZCRAFT_PORTAL_CATALOG_CLIENT_SECRET",
)
IDENTIFIER_PATTERN = re.compile(r"^[A-Za-z0-9._-]{1,120}$")


def environment_value(raw_value: str) -> str:
    values = shlex.split(raw_value, comments=True, posix=True)
    if len(values) != 1:
        raise ValueError("environment value must contain exactly one shell-style value")
    return values[0]


def read_environment(path: pathlib.Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for line_number, raw_line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line.removeprefix("export ").lstrip()
        key, separator, raw_value = line.partition("=")
        if key not in REQUIRED_ENVIRONMENT_KEYS:
            continue
        if not separator:
            raise ValueError(f"{path}:{line_number} is not a KEY=VALUE assignment")
        if key in values:
            raise ValueError(f"{path}:{line_number} duplicates {key}")
        try:
            values[key] = environment_value(raw_value)
        except ValueError as error:
            raise ValueError(f"{path}:{line_number} has an invalid {key}: {error}") from error

    missing = [key for key in REQUIRED_ENVIRONMENT_KEYS if not values.get(key)]
    if missing:
        raise ValueError("environment file is missing " + ", ".join(missing))
    if not IDENTIFIER_PATTERN.fullmatch(values["QUIZCRAFT_PORTAL_CATALOG_CLIENT_ID"]):
        raise ValueError("QUIZCRAFT_PORTAL_CATALOG_CLIENT_ID is not a portable service identifier")
    if not IDENTIFIER_PATTERN.fullmatch(values["QUIZCRAFT_PORTAL_CATALOG_KEY_ID"]):
        raise ValueError("QUIZCRAFT_PORTAL_CATALOG_KEY_ID is not a portable key identifier")
    secret = values["QUIZCRAFT_PORTAL_CATALOG_CLIENT_SECRET"]
    if len(secret) < 32 or any(character in secret for character in "\x00\r\n"):
        raise ValueError("QUIZCRAFT_PORTAL_CATALOG_CLIENT_SECRET is not a valid Portal-read secret")
    return values


def validate_request_uri(value: str) -> str:
    parsed = urllib.parse.urlsplit(value)
    if not value.startswith("/") or parsed.scheme or parsed.netloc or parsed.fragment or any(character in value for character in "\x00\r\n"):
        raise ValueError("request URI must be an absolute-path query without a host or fragment")
    return value


def curl_quote(value: str) -> str:
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'


def make_config(values: dict[str, str], request_uri: str) -> str:
    timestamp = str(int(time.time()))
    nonce = base64.urlsafe_b64encode(os.urandom(24)).decode().rstrip("=")
    canonical = "\n".join(
        [
            "GET",
            request_uri,
            timestamp,
            nonce,
            hashlib.sha256(b"").hexdigest(),
        ]
    )
    secret_value = values["QUIZCRAFT_PORTAL_CATALOG_CLIENT_SECRET"]
    signature = base64.urlsafe_b64encode(hmac.new(secret_value.encode(), canonical.encode(), hashlib.sha256).digest()).decode().rstrip("=")
    client_id = values["QUIZCRAFT_PORTAL_CATALOG_CLIENT_ID"]
    key_id = values["QUIZCRAFT_PORTAL_CATALOG_KEY_ID"]
    headers = (
        ("X-Service-Id", client_id),
        ("X-Key-Id", key_id),
        ("X-Timestamp", timestamp),
        ("X-Nonce", nonce),
        ("X-Signature", signature),
        ("X-Permission-Code", "portal.practice.read"),
        ("X-Scope-Kind", "product"),
        ("X-Product-Code", "quizcraft"),
        ("X-Request-Id", "req_cutover_ranking_" + secrets.token_hex(12)),
    )
    lines = [f"user = {curl_quote(client_id + ':' + secret_value)}"]
    lines.extend(f"header = {curl_quote(name + ': ' + value)}" for name, value in headers)
    return "\n".join(lines) + "\n"


def write_config(path: pathlib.Path, config: str) -> None:
    file_descriptor, temporary_path = tempfile.mkstemp(prefix=".portal-read.", dir=path.parent)
    try:
        os.fchmod(file_descriptor, 0o600)
        with os.fdopen(file_descriptor, "w", encoding="utf-8") as stream:
            stream.write(config)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary_path, path)
        os.chmod(path, 0o600)
    finally:
        if os.path.exists(temporary_path):
            os.unlink(temporary_path)


def main(arguments: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", required=True, type=pathlib.Path)
    parser.add_argument("--request-uri", required=True)
    parser.add_argument("--output", required=True, type=pathlib.Path)
    options = parser.parse_args(arguments)
    try:
        request_uri = validate_request_uri(options.request_uri)
        values = read_environment(options.environment_file)
        write_config(options.output, make_config(values, request_uri))
    except (OSError, ValueError) as error:
        print(f"Portal-read request configuration failed: {error}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
