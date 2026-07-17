#!/usr/bin/env python3
from __future__ import annotations

import os
import urllib.parse


def _is_local_http_host(hostname: str | None) -> bool:
    host = (hostname or "").lower()
    return host == "localhost" or host == "::1" or host.startswith("127.")


def resolve_admin_api_base_url(
    explicit_url: str | None,
    *,
    env_var: str = "QUIZCRAFT_API_BASE_URL",
) -> str:
    value = (explicit_url or os.getenv(env_var) or "").strip().rstrip("/")
    if not value:
        raise SystemExit(f"Missing {env_var}. Set it or pass --api-base-url.")

    parsed = urllib.parse.urlsplit(value)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise SystemExit("--api-base-url must be an absolute HTTP(S) URL.")
    if parsed.scheme == "http" and not _is_local_http_host(parsed.hostname):
        raise SystemExit("Admin API requests that carry X-Admin-Token must use HTTPS unless targeting localhost.")
    return value
