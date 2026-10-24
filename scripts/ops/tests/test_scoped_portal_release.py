"""Portal successor acceptance through its real release CLI and external commands."""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tarfile
import unittest

import test_scoped_food_release as food

SOURCE = food.SOURCE.with_name("deploy-henukit-portal-successor.py")
PREVIOUS = "c" * 40
COHORT = ("portal", "portal-gateway", "food", "console-gateway", "quizcraft")


class PortalCLI(unittest.TestCase):
    def setUp(self):
        food.ReleaseCLI.setUp(self)
        if SOURCE.exists():
            shutil.copy(SOURCE, self.runtime / "bin" / SOURCE.name)
        self.previous_runtime = self.root / "retained-practice"
        (self.previous_runtime / "bin").mkdir(parents=True)
        for name in (food.SOURCE.name, "deploy-henukit-practice-scoped.py"):
            shutil.copy(food.SOURCE.with_name(name), self.previous_runtime / "bin" / name)
        (self.previous_runtime / "RELEASE_SHA").write_text(PREVIOUS + "\n")
        (self.previous_runtime / "docker-compose.henukit.release.yml").write_text("services: {}\n")
        state_path = self.root / "fake-state.json"
        state = json.loads(state_path.read_text())
        template = state["containers"]["henukit-portal-1"]
        import copy
        for service in COHORT:
            name = "henukit-" + service + "-1"
            item = state["containers"].setdefault(name, copy.deepcopy(template))
            item["Name"] = "/" + name
            item["Id"] = "previous-" + service
            item["Config"]["Labels"]["com.docker.compose.config-hash"] = "config-" + service
            tag = "henukit-" + service + ":" + PREVIOUS
            item["Config"]["Image"] = tag
            item["Image"] = "sha256:" + hashlib.sha256(tag.encode()).hexdigest()
            state["images"][tag] = item["Image"]
        state_path.write_text(json.dumps(state))
        self.previous = self.root / "var/lib/henukit-scoped-releases/practice-console" / PREVIOUS
        self.previous.mkdir(parents=True)
        self.previous_receipt = self.previous / "receipt.json"
        self.previous_receipt.write_text(json.dumps({
            "format": "henukit-practice-scoped-v1", "status": "active", "scope": list(COHORT),
            "sha": PREVIOUS, "baseline": food.OLD,
            "baseline_runtime": str(self.root / "opt/henukit-releases" / food.OLD),
            "runtime": str(self.previous_runtime), "deployed_at": "2026-10-21T07:38:23Z",
            "images": {s: state["images"]["henukit-" + s + ":" + PREVIOUS] for s in COHORT},
            "helper_sha256": self.digest(self.previous_runtime / "bin/deploy-henukit-practice-scoped.py"),
            "engine_sha256": self.digest(self.previous_runtime / "bin" / food.SOURCE.name),
            "candidate_compose_sha256": self.digest(self.previous_runtime / "docker-compose.henukit.release.yml"),
            "environment_sha256": "historic-before-authorized-mail-migration",
            "baseline_compose_sha256": self.digest(self.root / "opt/henukit-releases" / food.OLD / "docker-compose.henukit.release.yml"),
        }))
        self.previous_receipt.chmod(0o600)
        self.marker = self.root / "var/lib/henukit-actions-watch/scoped-food-release"
        self.marker.symlink_to(self.previous)
        fake = food.FAKE.replace("assert sorted(selected)==['food','portal','portal-gateway'], selected", "assert selected==['portal'], selected")
        fake = fake.replace("services['food']['environment']['SECRET']='changed'", "services['portal']['environment']['SECRET']='changed'")
        fake = fake.replace("if '/quiz' in url or '/study-api/' in url:code=404", "if '/quiz/' in url or url.endswith('/quiz') or '/study-api/' in url:code=404")
        for name in ("docker", "gh", "systemctl", "curl"):
            (self.root / "bin" / name).write_text(fake)
        self.repack()

    @staticmethod
    def digest(path):
        return hashlib.sha256(path.read_bytes()).hexdigest()

    def repack(self):
        path = self.artifacts / ("henukit-runtime-" + food.NEW + ".tar.gz")
        with tarfile.open(path, "w:gz") as archive:
            for entry in self.runtime.rglob("*"):
                if entry.is_file():
                    archive.add(entry, arcname="./" + str(entry.relative_to(self.runtime)))
        Path(str(path) + ".sha256").write_text(self.digest(path) + "  " + path.name + "\n")
        (self.proof / ("henukit-getwork-actions-" + food.NEW + ".manifest")).write_text("artifact_sha256=" + self.digest(path) + "  " + path.name + "\n")

    def run_cli(self, mode, extra=None):
        return subprocess.run([
            "python3", str(self.runtime / "bin" / SOURCE.name), "--sha", food.NEW,
            "--run-id", "123", "--proof-dir", str(self.proof), "--test-root", str(self.root),
            "--acknowledge-getwork-deferred", mode,
        ], env={**self.env, **(extra or {})}, capture_output=True, text=True)

    def calls(self):
        return food.ReleaseCLI.calls(self)

    @property
    def record(self):
        return self.root / "var/lib/henukit-scoped-releases/portal-successor" / food.NEW / "receipt.json"

    def test_preflight_preserves_current_mail_and_practice_receipt_without_mutation(self):
        before = self.previous_receipt.read_bytes(), self.envfile.read_bytes()
        result = self.run_cli("--preflight")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(before, (self.previous_receipt.read_bytes(), self.envfile.read_bytes()))
        self.assertEqual(self.marker.resolve(), self.previous)
        self.assertFalse(self.record.exists())
        self.assertFalse(any(c[0] == "docker" and any(a in c[1] for a in ("load", "up", "exec")) for c in self.calls()))

    def portal_predecessor(self, receipt_format="henukit-portal-successor-v1", sha="d" * 40):
        previous_record = self.marker.resolve() / "receipt.json"
        previous = json.loads(previous_record.read_text())
        previous_runtime = Path(previous["runtime"])
        runtime = self.root / ("retained-portal-" + sha)
        (runtime / "bin").mkdir(parents=True)
        for name in (SOURCE.name, food.SOURCE.name):
            shutil.copy(self.runtime / "bin" / name, runtime / "bin" / name)
        (runtime / "RELEASE_SHA").write_text(sha + "\n")
        shutil.copy(self.runtime / "docker-compose.henukit.release.yml", runtime)
        state_path = self.root / "fake-state.json"
        state = json.loads(state_path.read_text())
        item = state["containers"]["henukit-portal-1"]

        def snapshot(containers):
            return {name: {
                "id": c["Id"], "image": c["Image"], "tag": c["Config"]["Image"],
                "status": c["State"]["Status"], "running": c["State"]["Running"],
                "restarts": c["RestartCount"], "started_at": c["State"].get("StartedAt", ""),
                "finished_at": c["State"].get("FinishedAt", ""), "exit_code": c["State"].get("ExitCode", 0),
                "pid": c["State"].get("Pid", 0),
            } for name, c in containers.items()}

        selected = snapshot({"henukit-portal-1": item})
        tag = "henukit-portal:" + sha
        image = "sha256:" + hashlib.sha256(tag.encode()).hexdigest()
        item.update({"Id": "previous-portal-" + sha, "Image": image})
        item["Config"]["Image"] = tag
        state["images"][tag] = image
        state_path.write_text(json.dumps(state))
        folder = self.root / "var/lib/henukit-scoped-releases/portal-successor" / sha
        folder.mkdir(parents=True)
        backup = folder / "configuration.backup"
        shutil.copy(self.envfile, backup)
        backup.chmod(0o600)
        record = folder / "receipt.json"
        record.write_text(json.dumps({
            "format": receipt_format, "status": "active", "scope": ["portal"], "sha": sha,
            "run_id": "122", "baseline": food.OLD, "baseline_runtime": previous["baseline_runtime"],
            "runtime": str(runtime), "deployed_at": "2026-10-24T01:00:00Z",
            "baseline_compose_sha256": previous["baseline_compose_sha256"],
            "helper_sha256": self.digest(runtime / "bin" / SOURCE.name),
            "engine_sha256": self.digest(runtime / "bin" / food.SOURCE.name),
            "candidate_compose_sha256": self.digest(runtime / "docker-compose.henukit.release.yml"),
            "predecessor_record": str(previous_record),
            "predecessor_receipt_sha256": self.digest(previous_record),
            "predecessor_runtime": str(previous_runtime), "predecessor_sha": previous["sha"],
            "predecessor_compose_sha256": previous["candidate_compose_sha256"],
            "old_version": previous["sha"], "old_deployed_at": previous["deployed_at"],
            "selected": selected,
            "unselected": snapshot({n: c for n, c in state["containers"].items() if n != "henukit-portal-1"}),
            "images": {"portal": image}, "environment_sha256": self.digest(self.envfile),
            "configuration_backup_sha256": self.digest(backup),
        }))
        record.chmod(0o600)
        self.marker.unlink()
        self.marker.symlink_to(folder)
        return record

    def test_preflight_accepts_retained_portal_without_mutating_either_receipt(self):
        previous = self.portal_predecessor()
        snapshots = {p: p.read_bytes() for p in (previous, self.previous_receipt, self.envfile)}
        result = self.run_cli("--preflight")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.marker.resolve(), previous.parent)
        self.assertFalse(self.record.exists())
        self.assertEqual(snapshots, {p: p.read_bytes() for p in snapshots})
        self.assert_no_replacement()

    def test_consecutive_v2_activation_and_repeated_rollback_preserve_the_entire_chain(self):
        first = self.portal_predecessor()
        previous = self.portal_predecessor("henukit-portal-successor-v2", "e" * 40)
        retained = {p: p.read_bytes() for p in (first, previous, self.previous_receipt, self.envfile)}
        before = json.loads((self.root / "fake-state.json").read_text())["containers"]
        for expected in ("active", "already_active"):
            result = self.run_cli("--execute")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout)["status"], expected)
        receipt = json.loads(self.record.read_text())
        self.assertEqual(receipt["format"], "henukit-portal-successor-v2")
        self.assertEqual(receipt["predecessor_sha"], "e" * 40)
        self.assertEqual(self.marker.resolve(), self.record.parent)
        state = json.loads((self.root / "fake-state.json").read_text())
        for name, original in before.items():
            if name != "henukit-portal-1":
                self.assertEqual(state["containers"][name], original, name)
        self.assertEqual(state["containers"]["henukit-portal-1"]["Config"]["Image"], "henukit-portal:" + food.NEW)
        receipt["status"] = "applying"
        self.record.write_text(json.dumps(receipt))
        for expected in ("rolled_back", "already_rolled_back"):
            result = self.run_cli("--rollback")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout)["status"], expected)
        self.assertEqual(self.marker.resolve(), previous.parent)
        state = json.loads((self.root / "fake-state.json").read_text())
        restored = state["containers"]["henukit-portal-1"]
        original = before["henukit-portal-1"]
        self.assertNotEqual(restored["Id"], original["Id"])
        self.assertEqual({k: v for k, v in restored.items() if k != "Id"},
                         {k: v for k, v in original.items() if k != "Id"})
        self.assertEqual(retained, {p: p.read_bytes() for p in retained})
        self.assertEqual(sum(c[0] == "docker" and "up" in c[1] for c in self.calls()), 2)
        self.assertFalse(any(c[0] == "docker" and "exec" in c[1] for c in self.calls()))

    def test_failed_consecutive_acceptance_restores_immediately_previous_portal(self):
        previous = self.portal_predecessor()
        retained = {p: p.read_bytes() for p in (previous, self.previous_receipt, self.envfile)}
        result = self.run_cli("--execute", {"FAIL_SMOKE": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("public acceptance failed", result.stderr)
        self.assertEqual(json.loads(self.record.read_text())["status"], "rolled_back")
        self.assertEqual(self.marker.resolve(), previous.parent)
        state = json.loads((self.root / "fake-state.json").read_text())
        self.assertEqual(state["containers"]["henukit-portal-1"]["Config"]["Image"], "henukit-portal:" + "d" * 40)
        self.assertEqual(retained, {p: p.read_bytes() for p in retained})

    def test_consecutive_preflight_rejects_full_baseline_compose_drift(self):
        self.portal_predecessor()
        baseline = self.root / "opt/henukit-releases" / food.OLD / "docker-compose.henukit.release.yml"
        baseline.write_text(baseline.read_text() + "# changed baseline\n")
        result = self.run_cli("--preflight")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("retained full baseline Compose changed", result.stderr)
        self.assert_no_replacement()

    def test_consecutive_preflight_rejects_missing_or_retagged_ancestor_images(self):
        self.portal_predecessor()
        self.portal_predecessor("henukit-portal-successor-v2", "e" * 40)
        state_path = self.root / "fake-state.json"
        state = json.loads(state_path.read_text())
        tag = "henukit-portal:" + "d" * 40
        for missing in (False, True):
            with self.subTest(missing=missing):
                if missing:
                    state["images"].pop(tag)
                else:
                    state["images"][tag] = "sha256:" + "0" * 64
                state_path.write_text(json.dumps(state))
                result = self.run_cli("--preflight")
                self.assertNotEqual(result.returncode, 0)
                self.assert_no_replacement()

    def test_consecutive_preflight_rejects_retained_file_and_non_portal_drift(self):
        previous = self.portal_predecessor()
        data = json.loads(previous.read_text())
        for path in (
            Path(data["runtime"]) / "bin" / SOURCE.name,
            Path(data["runtime"]) / "docker-compose.henukit.release.yml",
            previous.parent / "configuration.backup",
            self.previous_receipt,
        ):
            original = path.read_bytes()
            path.write_bytes(original + b"\nchanged\n")
            result = self.run_cli("--preflight")
            self.assertNotEqual(result.returncode, 0, path)
            path.write_bytes(original)
        state_path = self.root / "fake-state.json"
        state = json.loads(state_path.read_text())
        state["containers"]["henukit-nginx-1"]["Id"] = "foreign-replacement"
        state_path.write_text(json.dumps(state))
        self.assertNotEqual(self.run_cli("--preflight").returncode, 0)
        self.assert_no_replacement()

    def test_consecutive_preflight_rejects_cycles_and_outside_parent_paths(self):
        previous = self.portal_predecessor()
        original = json.loads(previous.read_text())
        for path, sha in ((str(previous), "d" * 40), (str(self.root / "foreign/receipt.json"), PREVIOUS)):
            data = {**original, "predecessor_record": path, "predecessor_sha": sha}
            previous.write_text(json.dumps(data))
            self.assertNotEqual(self.run_cli("--preflight").returncode, 0, path)
        self.assert_no_replacement()

    def test_consecutive_preflight_rejects_unbounded_history(self):
        for index in range(1, 65):
            self.portal_predecessor("henukit-portal-successor-v2", f"{index:040x}")
        result = self.run_cli("--preflight")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("exceeds 64", result.stderr)
        self.assert_no_replacement()

    def test_execute_only_portal_and_repeat_without_another_replacement(self):
        before = json.loads((self.root / "fake-state.json").read_text())["containers"]
        receipt_before, env_before = self.previous_receipt.read_bytes(), self.envfile.read_bytes()
        for expected in ("active", "already_active"):
            result = self.run_cli("--execute")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout)["status"], expected)
        after = json.loads((self.root / "fake-state.json").read_text())["containers"]
        for name in before:
            if name != "henukit-portal-1":
                self.assertEqual(before[name], after[name], name)
        self.assertEqual(after["henukit-portal-1"]["Config"]["Image"], "henukit-portal:" + food.NEW)
        self.assertEqual(receipt_before, self.previous_receipt.read_bytes())
        self.assertEqual(env_before, self.envfile.read_bytes())
        self.assertEqual((self.record.parent / "configuration.backup").read_bytes(), env_before)
        self.assertEqual(self.marker.resolve(), self.record.parent)
        self.assertEqual((self.root / "opt/henukit/current").resolve().name, food.OLD)
        calls = self.calls()
        self.assertEqual(sum(c[0] == "docker" and "up" in c[1] for c in calls), 1)
        self.assertFalse(any(c[0] == "docker" and "exec" in c[1] for c in calls))
        self.assertFalse(any(c[0] == "curl" and "PUT" in c[1] for c in calls))

    def assert_restored(self, previous_bytes):
        state = json.loads((self.root / "fake-state.json").read_text())
        self.assertEqual(state["containers"]["henukit-portal-1"]["Config"]["Image"], "henukit-portal:" + PREVIOUS)
        self.assertEqual(self.marker.resolve(), self.previous)
        self.assertEqual(self.previous_receipt.read_bytes(), previous_bytes)
        self.assertEqual(json.loads(self.record.read_text())["status"], "rolled_back")

    def test_failed_acceptance_restores_previous_portal_and_practice_marker(self):
        previous_bytes = self.previous_receipt.read_bytes()
        result = self.run_cli("--execute", {"FAIL_SMOKE": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("public acceptance failed", result.stderr)
        self.assert_restored(previous_bytes)

    def test_explicit_interrupted_rollback_and_repeat_use_previous_scoped_image(self):
        previous_bytes = self.previous_receipt.read_bytes()
        result = self.run_cli("--execute")
        self.assertEqual(result.returncode, 0, result.stderr)
        data = json.loads(self.record.read_text())
        data["status"] = "applying"
        self.record.write_text(json.dumps(data))
        for expected in ("rolled_back", "already_rolled_back"):
            result = self.run_cli("--rollback")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout)["status"], expected)
        self.assert_restored(previous_bytes)
        self.assertEqual(sum(c[0] == "docker" and "up" in c[1] for c in self.calls()), 2)

    def assert_no_replacement(self):
        self.assertFalse(any(c[0] == "docker" and any(a in c[1] for a in ("up", "load", "exec")) for c in self.calls()))

    def test_unknown_or_inactive_predecessor_and_wrong_image_fail_closed(self):
        original = self.previous_receipt.read_bytes()
        for change in ({"format": "unknown"}, {"scope": ["portal"]}, {"status": "applying"}, {"images": {s: "wrong" for s in COHORT}}):
            data = json.loads(original)
            data.update(change)
            self.previous_receipt.write_text(json.dumps(data))
            result = self.run_cli("--execute")
            self.assertNotEqual(result.returncode, 0, change)
        self.assert_no_replacement()
        self.assertEqual(self.marker.resolve(), self.previous)

    def test_predecessor_runtime_and_rollback_image_cannot_drift(self):
        for relative in ("docker-compose.henukit.release.yml", "bin/deploy-henukit-practice-scoped.py", "bin/" + food.SOURCE.name):
            path = self.previous_runtime / relative
            original = path.read_bytes()
            path.write_bytes(original + b"\n# drift\n")
            self.assertNotEqual(self.run_cli("--preflight").returncode, 0, relative)
            path.write_bytes(original)
        path = self.root / "fake-state.json"
        state = json.loads(path.read_text())
        state["images"]["henukit-portal:" + PREVIOUS] = "retagged"
        path.write_text(json.dumps(state))
        result = self.run_cli("--execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("rollback image", result.stderr)
        self.assert_no_replacement()

    def test_source_provenance_and_incompatible_config_stop_before_mutation(self):
        for override in ({"STALE_SOURCE": "1"}, {"BAD_PROVENANCE": "1"}, {"BAD_CONFIG": "1"}):
            result = self.run_cli("--execute", override)
            self.assertNotEqual(result.returncode, 0, override)
        self.assert_no_replacement()

    def test_archive_and_executed_entry_tampering_are_rejected(self):
        path = self.artifacts / ("henukit-portal-" + food.NEW + ".docker.tar.gz")
        original = path.read_bytes()
        path.write_bytes(b"tampered")
        result = self.run_cli("--execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("checksum mismatch", result.stderr)
        path.write_bytes(original)
        entry = self.runtime / "bin" / SOURCE.name
        entry.write_text(entry.read_text() + "\n# drift\n")
        result = self.run_cli("--execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("scope entry differs", result.stderr)
        self.assert_no_replacement()

    def test_pending_full_release_blocks_preflight(self):
        path = self.root / "var/lib/henukit-actions-watch/approvals/pending"
        path.write_text("pending")
        result = self.run_cli("--execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("pending full release", result.stderr)
        self.assert_no_replacement()

    def test_running_portal_environment_and_config_hash_are_bound(self):
        path = self.root / "fake-state.json"
        original = path.read_text()
        for field in ("Env", "Labels"):
            state = json.loads(original)
            config = state["containers"]["henukit-portal-1"]["Config"]
            if field == "Env":
                config["Env"][0] = "SECRET=changed"
            else:
                config["Labels"]["com.docker.compose.config-hash"] = "changed"
            path.write_text(json.dumps(state))
            result = self.run_cli("--execute")
            self.assertNotEqual(result.returncode, 0, field)
            self.assertIn("running configuration", result.stderr)
        self.assert_no_replacement()

    def test_unselected_drift_keeps_blocking_marker_without_unsafe_repair(self):
        result = self.run_cli("--execute", {"CHANGE_UNSELECTED": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("rollback failed", result.stderr)
        self.assertEqual(json.loads(self.record.read_text())["status"], "rollback_failed")
        self.assertEqual(self.marker.resolve(), self.record.parent)
        self.assertEqual(sum(c[0] == "docker" and "up" in c[1] for c in self.calls()), 1)

    def test_rollback_refuses_config_receipt_compose_or_marker_drift(self):
        self.assertEqual(self.run_cli("--execute").returncode, 0)
        for path in (self.envfile, self.previous_receipt, self.previous_runtime / "docker-compose.henukit.release.yml"):
            original = path.read_bytes()
            path.write_bytes(original + b"\nchanged\n")
            result = self.run_cli("--rollback")
            self.assertNotEqual(result.returncode, 0, path.name)
            path.write_bytes(original)
        self.marker.unlink()
        self.marker.symlink_to(self.root / "foreign")
        self.assertNotEqual(self.run_cli("--rollback").returncode, 0)
        self.assertEqual(sum(c[0] == "docker" and "up" in c[1] for c in self.calls()), 1)

    def test_watcher_safe_boundary_is_restored_after_activation(self):
        path = self.root / "fake-state.json"
        state = json.loads(path.read_text())
        state["watcher"] = True
        path.write_text(json.dumps(state))
        instance = self.root / "var/lib/henukit-actions-watch/watcher.instance"
        instance.write_text("123\n")
        instance.chmod(0o600)
        result = self.run_cli("--execute")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(json.loads(path.read_text())["watcher"])
        self.assertFalse((instance.parent / "quiesce.request").exists())

    def test_interrupted_backup_intent_can_finish_without_replacing_original_portal(self):
        state_file = self.root / "fake-state.json"
        original_state = state_file.read_bytes()
        self.assertEqual(self.run_cli("--execute").returncode, 0)
        # Model the durable preparing journal left by a kill before backup
        # completion. No image replacement or marker advance has happened.
        state_file.write_bytes(original_state)
        self.marker.unlink()
        self.marker.symlink_to(self.previous)
        data = json.loads(self.record.read_text())
        data["status"] = "preparing"
        data.pop("configuration_backup_sha256")
        data.pop("deployed_at")
        data["images"] = {}
        self.record.write_text(json.dumps(data))
        (self.record.parent / "configuration.backup").write_bytes(b"partial")
        (self.root / "calls.jsonl").unlink()
        for expected in ("rolled_back", "already_rolled_back"):
            result = self.run_cli("--rollback")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout)["status"], expected)
        self.assert_no_replacement()
        self.assertEqual(self.marker.resolve(), self.previous)


if __name__ == "__main__":
    unittest.main()
