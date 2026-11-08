"""The root coordinator accepts only the current private fixture's exact FD."""

import array
import json
import os
from pathlib import Path
import socket
import tempfile
import unittest

import run_stack_root_gate as gate


class ForeignOwnerRequest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="henukit-scoped-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name).resolve()
        self.config = self.root / "etc/henukit-deploy/deploy.env"
        self.config.parent.mkdir(parents=True)
        self.config.write_text("HENUKIT_DEPLOY_LOCK_FILE=fixture-only\n")
        self.config.chmod(0o640)

    def reject(self, descriptor, path):
        parent, child = socket.socketpair()
        self.addCleanup(parent.close)
        self.addCleanup(child.close)
        child.sendmsg([json.dumps({"path": str(path)}).encode()], [
            (socket.SOL_SOCKET, socket.SCM_RIGHTS, array.array("i", [descriptor]))])
        before = os.fstat(descriptor)
        with self.assertRaises(RuntimeError):
            gate.accept_request(parent, os.geteuid(), os.getegid())
        after = os.fstat(descriptor)
        self.assertEqual((after.st_uid, after.st_gid, after.st_mode),
                         (before.st_uid, before.st_gid, before.st_mode))

    def test_another_inode_cannot_be_selected_by_path(self):
        other = self.config.with_name("other")
        other.write_bytes(self.config.read_bytes())
        other.chmod(0o640)
        with other.open("rb") as stream:
            self.reject(stream.fileno(), self.config)

    def test_a_linked_config_is_refused_before_chown(self):
        physical = self.config.with_name("physical")
        self.config.rename(physical)
        self.config.symlink_to(physical)
        with physical.open("rb") as stream:
            self.reject(stream.fileno(), self.config)

    def test_a_different_config_mode_is_refused_before_chown(self):
        self.config.chmod(0o644)
        with self.config.open("rb") as stream:
            self.reject(stream.fileno(), self.config)

    def test_a_writable_fixture_parent_is_refused_before_chown(self):
        self.config.parent.chmod(0o777)
        try:
            with self.config.open("rb") as stream:
                self.reject(stream.fileno(), self.config)
        finally:
            self.config.parent.chmod(0o755)

    def test_a_hardlinked_config_is_refused_before_chown(self):
        other = self.config.with_name("hardlink")
        os.link(self.config, other)
        with self.config.open("rb") as stream:
            self.reject(stream.fileno(), self.config)

    def test_an_unrelated_file_cannot_be_changed(self):
        with tempfile.NamedTemporaryFile() as unrelated:
            os.fchmod(unrelated.fileno(), 0o640)
            self.reject(unrelated.fileno(), Path(unrelated.name))


if __name__ == "__main__":
    unittest.main()
