"""Use real root chown for one runner-owned CLI fixture, never a host path.

Historical scoped CLIs intentionally forbid root --test-root. Keep that rule:
the existing runner executes their fixture and root only changes its exact FD.
"""

import array
import json
import os
from pathlib import Path
import pwd
import socket
import stat
import sys
import tempfile
import unittest

TEST = "test_stack_transition.StackTransitionCheckpoint.test_source_config_foreign_owner_is_refused"


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def identity(info):
    return (info.st_dev, info.st_ino, info.st_uid, info.st_gid,
            stat.S_IMODE(info.st_mode), info.st_size, info.st_mtime_ns)


def accept_request(channel, uid, gid):
    raw, ancillary, flags, _ = channel.recvmsg(4096, socket.CMSG_SPACE(8))
    descriptors = array.array("i")
    try:
        for level, kind, value in ancillary:
            require((level, kind) == (socket.SOL_SOCKET, socket.SCM_RIGHTS),
                    "unexpected fixture request metadata")
            descriptors.frombytes(value)
        require(not flags and len(descriptors) == 1, "one complete fixture FD is required")
        request = json.loads(raw)
        require(isinstance(request, dict) and set(request) == {"path"}
                and isinstance(request["path"], str), "invalid fixture request")
        path = Path(request["path"])
        require(path.is_absolute() and len(path.parents) >= 3, "fixture path must be absolute")
        root = path.parents[2]
        require(path == root / "etc/henukit-deploy/deploy.env"
                and root.parent == Path(tempfile.gettempdir()).resolve()
                and root.name.startswith("henukit-scoped-"), "not the scoped test configuration")
        for parent in (root, path.parent.parent, path.parent):
            info = parent.lstat()
            require(stat.S_ISDIR(info.st_mode) and info.st_uid == uid
                    and not info.st_mode & 0o022, "unsafe fixture parent")
        require(stat.S_IMODE(root.lstat().st_mode) == 0o700, "fixture must be private")
        descriptor = descriptors[0]
        before = os.fstat(descriptor)
        require(stat.S_ISREG(before.st_mode) and before.st_nlink == 1
                and (before.st_uid, before.st_gid, stat.S_IMODE(before.st_mode)) == (uid, gid, 0o640)
                and 0 < before.st_size <= 8192
                and identity(path.lstat()) == identity(before), "fixture FD/profile differs")
        os.fchown(descriptor, 0, gid)
        after = os.fstat(descriptor)
        require(after.st_uid == 0 and after.st_gid == gid and after.st_nlink == 1
                and stat.S_IMODE(after.st_mode) == 0o640
                and (after.st_dev, after.st_ino) == (before.st_dev, before.st_ino)
                and identity(path.lstat()) == identity(after), "foreign fixture ownership did not persist")
        return {"fixture_uid_before": uid, "fixture_uid_after": after.st_uid,
                "fixture_gid": gid, "actual_foreign_owner": uid != after.st_uid}
    finally:
        for descriptor in descriptors:
            os.close(descriptor)


def run_fixture(channel, uid, gid):
    os.setgroups([])
    os.setgid(gid)
    os.setuid(uid)
    require(os.getuid() == os.geteuid() == uid and os.getgid() == os.getegid() == gid,
            "runner identity was not retained")
    original_chown = os.chown

    def fixture_chown(path, owner, group, *args, **kwargs):
        try:
            return original_chown(path, owner, group, *args, **kwargs)
        except PermissionError:
            if owner != 0 or group != gid or args or kwargs:
                raise
        descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
        try:
            channel.sendmsg([json.dumps({"path": os.fspath(path)}).encode()], [
                (socket.SOL_SOCKET, socket.SCM_RIGHTS, array.array("i", [descriptor]))])
            require(channel.recv(16) == b"changed", "root fixture preparation failed")
            changed = os.fstat(descriptor)
            require(changed.st_uid == 0 and changed.st_gid == gid,
                    "the fixture did not obtain an actual foreign owner")
        finally:
            os.close(descriptor)

    os.chown = fixture_chown
    try:
        result = unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromName(TEST))
        return 0 if result.wasSuccessful() and result.testsRun == 1 and not result.skipped else 1
    finally:
        os.chown = original_chown


def main():
    require(len(sys.argv) == 1 and sys.platform == "linux" and os.getuid() == os.geteuid() == 0,
            "this fixed CI gate requires Linux root")
    require(os.environ.get("HENUKIT_REQUIRE_FOREIGN_OWNER") == "1", "foreign-owner gate is required")
    uid, gid = int(os.environ["SUDO_UID"]), int(os.environ["SUDO_GID"])
    caller = pwd.getpwuid(uid)
    require(uid > 0 and gid > 0 and caller.pw_gid == gid
            and caller.pw_name == os.environ["SUDO_USER"], "original nonroot sudo runner is required")
    parent, child = socket.socketpair()
    pid = os.fork()
    if pid == 0:
        parent.close()
        code = 1
        try:
            code = run_fixture(child, uid, gid)
        finally:
            child.close()
            sys.stdout.flush()
            sys.stderr.flush()
        os._exit(code)
    child.close()
    parent.settimeout(90)
    proof = None
    try:
        proof = accept_request(parent, uid, gid)
        parent.sendall(b"changed")
    finally:
        parent.close()
        _, status = os.waitpid(pid, 0)
    require(proof is not None and proof["actual_foreign_owner"]
            and os.waitstatus_to_exitcode(status) == 0, "required CLI ownership gate failed")
    print(json.dumps({"gate": TEST, **proof, "passed": True}))


if __name__ == "__main__":
    main()
