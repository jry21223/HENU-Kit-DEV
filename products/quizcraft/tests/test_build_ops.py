import os
import shutil
import stat
import subprocess
from pathlib import Path

import pytest


def test_build_ops_rejects_unapproved_static_dir_with_portable_error(tmp_path):
    fake_bin = tmp_path / "bin"
    fake_bin.mkdir()
    fake_npm = fake_bin / "npm"
    fake_npm.write_text(
        """#!/bin/sh
mkdir -p dist/assets
printf '<html></html>\\n' > dist/index.html
printf 'bundle\\n' > dist/assets/index.js
""",
        encoding="utf-8",
    )
    fake_npm.chmod(fake_npm.stat().st_mode | stat.S_IXUSR)

    env = os.environ.copy()
    env["PATH"] = f"{fake_bin}:{env['PATH']}"
    env["STATIC_DEPLOY_DIR"] = str(tmp_path / "outside-approved-root")

    result = subprocess.run(
        ["bash", "scripts/build_ops.sh"],
        cwd=Path(__file__).resolve().parents[1],
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )

    assert result.returncode == 1
    assert "refusing to deploy outside approved static roots" in result.stderr
    assert "illegal option" not in result.stderr


def test_build_ops_accepts_documented_static_root_before_directory_check(tmp_path):
    if Path("/var/www/quizcraft-cn").exists():
        pytest.skip("documented production static root exists on this machine")

    fake_bin = tmp_path / "bin"
    fake_bin.mkdir()
    fake_npm = fake_bin / "npm"
    fake_npm.write_text(
        """#!/bin/sh
mkdir -p dist/assets
printf '<html></html>\\n' > dist/index.html
printf 'bundle\\n' > dist/assets/index.js
""",
        encoding="utf-8",
    )
    fake_npm.chmod(fake_npm.stat().st_mode | stat.S_IXUSR)

    env = os.environ.copy()
    env["PATH"] = f"{fake_bin}:{env['PATH']}"
    env["STATIC_DEPLOY_DIR"] = "/var/www/quizcraft-cn"

    result = subprocess.run(
        ["bash", "scripts/build_ops.sh"],
        cwd=Path(__file__).resolve().parents[1],
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )

    assert result.returncode == 1
    assert "static deploy dir does not exist: /var/www/quizcraft-cn" in result.stderr
    assert "refusing to deploy outside approved static roots" not in result.stderr


def test_build_ops_static_dir_validation_does_not_require_python(tmp_path):
    fake_bin = tmp_path / "bin"
    fake_bin.mkdir()
    fake_npm = fake_bin / "npm"
    fake_npm.write_text(
        """#!/bin/sh
mkdir -p dist/assets
printf '<html></html>\\n' > dist/index.html
printf 'bundle\\n' > dist/assets/index.js
""",
        encoding="utf-8",
    )
    fake_npm.chmod(fake_npm.stat().st_mode | stat.S_IXUSR)

    for tool in ("bash", "cat", "cp", "dirname", "mkdir", "pwd", "rm"):
        source = shutil.which(tool)
        if source:
            (fake_bin / tool).symlink_to(source)

    fake_python = fake_bin / "python3"
    fake_python.write_text(
        """#!/bin/sh
echo 'python3 should not be used' >&2
exit 127
""",
        encoding="utf-8",
    )
    fake_python.chmod(fake_python.stat().st_mode | stat.S_IXUSR)

    deploy_dir = tmp_path / "var" / "www" / "quizcraft-cn"
    deploy_dir.mkdir(parents=True)

    env = os.environ.copy()
    env["PATH"] = str(fake_bin)
    env["STATIC_DEPLOY_DIR"] = str(deploy_dir)

    result = subprocess.run(
        ["bash", "scripts/build_ops.sh"],
        cwd=Path(__file__).resolve().parents[1],
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )

    assert result.returncode == 1
    assert "python3 should not be used" not in result.stderr
    assert "refusing to deploy outside approved static roots" in result.stderr
