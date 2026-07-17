from __future__ import annotations

from pathlib import Path, PureWindowsPath
from typing import Any
from urllib.parse import unquote, urlparse


LOCAL_PATH_MARKERS = ("/Users/", "/home/", "C:\\Users\\", "file://")


def _filename_from_text(value: str) -> str:
    parsed = urlparse(value)
    if parsed.scheme == "file":
        value = unquote(parsed.path)

    if "\\" in value:
        return PureWindowsPath(value).name

    return Path(value).name


def looks_like_local_source_reference(value: str) -> bool:
    stripped = value.strip()
    if not stripped:
        return False
    if any(marker in stripped for marker in LOCAL_PATH_MARKERS):
        return True
    if stripped.startswith("/") and Path(stripped).suffix:
        return True
    return False


def sanitize_source_reference(value: str) -> str:
    stripped = value.strip()
    if not looks_like_local_source_reference(stripped):
        return value

    filename = _filename_from_text(stripped)
    return filename or value


def sanitize_source_metadata(value: Any) -> Any:
    if isinstance(value, dict):
        return {key: sanitize_source_metadata(item) for key, item in value.items()}

    if isinstance(value, list):
        return [sanitize_source_metadata(item) for item in value]

    if isinstance(value, str):
        return sanitize_source_reference(value)

    return value
