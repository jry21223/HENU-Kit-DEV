#!/usr/bin/env python3
"""Contract test for the Portal-read curl configuration writer."""

from __future__ import annotations

import base64
import hashlib
import hmac
import pathlib
import subprocess
import sys
import tempfile
import time


SCRIPT = pathlib.Path(__file__).with_name("write-portal-read-request-config.py")
REQUEST_URI = "/api/v1/rankings/overall?period=weekly"
CLIENT_ID = "portal-gateway"
KEY_ID = "portal-catalog-key-1"
SECRET = "portal-catalog-secret-at-least-32-bytes"


def curl_config_value(value: str) -> str:
    assert value.startswith('"') and value.endswith('"'), value
    return value[1:-1].replace(r"\\", "\\").replace(r'\"', '"')


def read_curl_config(path: pathlib.Path) -> tuple[str, dict[str, str]]:
    user = ""
    headers: dict[str, str] = {}
    for line in path.read_text().splitlines():
        name, separator, raw_value = line.partition("=")
        assert separator and name.strip() in {"user", "header"}, line
        value = curl_config_value(raw_value.strip())
        if name.strip() == "user":
            assert not user, "duplicate curl user"
            user = value
            continue
        header, separator, header_value = value.partition(": ")
        assert separator and header not in headers, value
        headers[header] = header_value
    return user, headers


def write_environment(path: pathlib.Path, include_secret: bool = True) -> None:
    lines = [
        f"QUIZCRAFT_PORTAL_CATALOG_CLIENT_ID={CLIENT_ID}",
        f"QUIZCRAFT_PORTAL_CATALOG_KEY_ID={KEY_ID}",
    ]
    if include_secret:
        lines.append(f"QUIZCRAFT_PORTAL_CATALOG_CLIENT_SECRET={SECRET}")
    path.write_text("\n".join(lines) + "\n")


def test_writes_a_valid_signed_portal_read_request() -> None:
    with tempfile.TemporaryDirectory() as temporary_directory:
        root = pathlib.Path(temporary_directory)
        environment_file = root / "quizcraft-go.env"
        output_file = root / "portal-read.curl"
        write_environment(environment_file)

        subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--environment-file",
                str(environment_file),
                "--request-uri",
                REQUEST_URI,
                "--output",
                str(output_file),
            ],
            check=True,
        )

        assert output_file.stat().st_mode & 0o077 == 0
        user, headers = read_curl_config(output_file)
        assert user == f"{CLIENT_ID}:{SECRET}"
        assert headers["X-Service-Id"] == CLIENT_ID
        assert headers["X-Key-Id"] == KEY_ID
        assert headers["X-Permission-Code"] == "portal.practice.read"
        assert headers["X-Scope-Kind"] == "product"
        assert headers["X-Product-Code"] == "quizcraft"
        assert headers["X-Request-Id"].startswith("req_cutover_ranking_")

        timestamp = int(headers["X-Timestamp"])
        assert abs(time.time() - timestamp) <= 5
        nonce = headers["X-Nonce"]
        assert len(base64.urlsafe_b64decode(nonce + "==")) == 24
        canonical = "\n".join(
            [
                "GET",
                REQUEST_URI,
                headers["X-Timestamp"],
                nonce,
                hashlib.sha256(b"").hexdigest(),
            ]
        )
        assert headers["X-Signature"] == base64.urlsafe_b64encode(
            hmac.new(SECRET.encode(), canonical.encode(), hashlib.sha256).digest()
        ).decode().rstrip("=")


def test_rejects_an_environment_file_without_the_portal_secret() -> None:
    with tempfile.TemporaryDirectory() as temporary_directory:
        root = pathlib.Path(temporary_directory)
        environment_file = root / "quizcraft-go.env"
        output_file = root / "portal-read.curl"
        write_environment(environment_file, include_secret=False)

        result = subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--environment-file",
                str(environment_file),
                "--request-uri",
                REQUEST_URI,
                "--output",
                str(output_file),
            ],
            text=True,
            capture_output=True,
        )

        assert result.returncode != 0
        assert "QUIZCRAFT_PORTAL_CATALOG_CLIENT_SECRET" in result.stderr
        assert not output_file.exists()


if __name__ == "__main__":
    test_writes_a_valid_signed_portal_read_request()
    test_rejects_an_environment_file_without_the_portal_secret()
    print("Portal-read curl configuration tests passed")
