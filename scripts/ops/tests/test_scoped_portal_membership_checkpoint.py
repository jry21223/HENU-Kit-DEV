"""Portal releases over a real membership CLI checkpoint."""

import json
from pathlib import Path
import shutil

import test_scoped_food_release as food
import test_scoped_membership_release as membership
import test_scoped_portal_release as portal

MEMBERSHIP_SHA = "f" * 40


class PortalMembershipCLI(membership.MembershipCLI):
    def setUp(self):
        super().setUp()
        self.portal_record = self.portal_predecessor()
        result = membership.MembershipCLI.run_cli(self, "--execute")
        self.assertEqual(result.returncode, 0, result.stderr)

        # Retain the completed owner release as a prior exact-source candidate.
        original = self.root / "var/lib/henukit-scoped-releases/account-membership" / food.NEW
        retained = original.with_name(MEMBERSHIP_SHA)
        shutil.copytree(original, retained)
        runtime = self.root / "retained-membership"
        shutil.copytree(self.runtime, runtime)
        (runtime / "RELEASE_SHA").write_text(MEMBERSHIP_SHA + "\n")
        for path in retained.iterdir():
            if path.name in ("receipt.json", "before.compose.json", "after.compose.json", "nginx.conf"):
                path.write_text(path.read_text().replace(str(original), str(retained)).replace(food.NEW, MEMBERSHIP_SHA))
        self.membership_record = retained / "receipt.json"
        data = json.loads(self.membership_record.read_text())
        data["runtime"] = str(runtime)
        data["configuration_hashes"] = {
            name: self.digest(retained / name) for name in data["configuration_hashes"]
        }
        self.membership_record.write_text(json.dumps(data))
        self.marker.unlink()
        self.marker.symlink_to(retained)
        state_path = self.root / "fake-state.json"
        original_state = json.loads(state_path.read_text())
        state = json.loads(state_path.read_text().replace(str(original), str(retained)).replace(food.NEW, MEMBERSHIP_SHA))
        state["images"].update(original_state["images"])
        owner = state["containers"]["henukit-account-portfolio-1"]
        owner["Config"]["Image"] = "henukit-account-portfolio:" + MEMBERSHIP_SHA
        state["images"][owner["Config"]["Image"]] = owner["Image"]
        state["deployed"] = False
        state_path.write_text(json.dumps(state))

        fake = food.FAKE.replace("unchanged", "unchanged$literal")
        fake = fake.replace("assert sorted(selected)==['food','portal','portal-gateway'], selected", "assert selected==['portal'], selected")
        fake = fake.replace("services['food']['environment']['SECRET']='changed'", "services['portal']['environment']['SECRET']='changed'")
        fake = fake.replace("def save():", "def literal(v):\n if isinstance(v,str):return v.replace('$$','$')\n if isinstance(v,list):return [literal(x) for x in v]\n if isinstance(v,dict):return {k:literal(x) for k,x in v.items()}\n return v\ndef save():")
        fake = fake.replace(" elif 'config' in args:", " elif 'config' in args and args[args.index('-f')+1].endswith('.compose.json'):out(literal(json.loads(pathlib.Path(args[args.index('-f')+1]).read_text())))\n  elif 'config' in args:")
        for name in ("docker", "gh", "systemctl", "curl"):
            (self.root / "bin" / name).write_text(fake)
        self.repack()

    def run_cli(self, mode, extra=None):
        return portal.PortalCLI.run_cli(self, mode, extra)

    @property
    def record(self):
        return portal.PortalCLI.record.fget(self)

    def test_membership_checkpoint_preflight_preserves_all_services_and_receipts(self):
        retained = {p: p.read_bytes() for p in (self.membership_record, self.portal_record, self.envfile)}
        before = (self.root / "fake-state.json").read_bytes()
        (self.root / "calls.jsonl").unlink()
        result = self.run_cli("--preflight")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.marker.resolve(), self.membership_record.parent)
        self.assertEqual(retained, {p: p.read_bytes() for p in retained})
        self.assertEqual(before, (self.root / "fake-state.json").read_bytes())
        self.assertFalse(self.record.exists())
        self.assert_no_replacement()

    def test_portal_activation_and_rollback_preserve_membership_checkpoint(self):
        retained = {p: p.read_bytes() for p in (self.membership_record, self.portal_record, self.envfile)}
        before = json.loads((self.root / "fake-state.json").read_text())["containers"]
        (self.root / "calls.jsonl").unlink()
        for expected in ("active", "already_active"):
            result = self.run_cli("--execute")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout)["status"], expected)
        data = json.loads(self.record.read_text())
        self.assertEqual(data["format"], "henukit-portal-successor-v3")
        self.assertEqual(data["membership_checkpoint"]["record"], str(self.membership_record))
        after = json.loads((self.root / "fake-state.json").read_text())["containers"]
        self.assertEqual(after["henukit-portal-1"]["Config"]["Image"], "henukit-portal:" + food.NEW)
        for name in before:
            if name != "henukit-portal-1":
                self.assertEqual(before[name], after[name], name)
        for expected in ("rolled_back", "already_rolled_back"):
            result = self.run_cli("--rollback")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout)["status"], expected)
        self.assertEqual(self.marker.resolve(), self.record.parent)
        self.assertEqual(json.loads(self.record.read_text())["status"], "rolled_back")
        restored = json.loads((self.root / "fake-state.json").read_text())["containers"]
        self.assertEqual(restored["henukit-portal-1"]["Config"]["Image"], before["henukit-portal-1"]["Config"]["Image"])
        self.assertEqual(retained, {p: p.read_bytes() for p in retained})
        self.assertEqual(sum(c[0] == "docker" and "up" in c[1] for c in self.calls()), 2)
        self.assertFalse(any(c[0] == "docker" and "exec" in c[1] for c in self.calls()))

    def test_failed_acceptance_restores_portal_and_retains_its_rollback_checkpoint(self):
        before = self.membership_record.read_bytes(), self.portal_record.read_bytes(), self.envfile.read_bytes()
        result = self.run_cli("--execute", {"FAIL_SMOKE": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("public acceptance failed", result.stderr)
        self.assertEqual(json.loads(self.record.read_text())["status"], "rolled_back")
        self.assertEqual(self.marker.resolve(), self.record.parent)
        self.assertEqual(before, (self.membership_record.read_bytes(), self.portal_record.read_bytes(), self.envfile.read_bytes()))
        result = self.run_cli("--rollback")
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_member_metadata_and_retained_files_cannot_be_substituted(self):
        original = self.membership_record.read_bytes()
        for change in ({"format": "unknown"}, {"scope": ["portal"]}, {"status": "applying"}, {"status": "rolled_back"}, {"selected_checkpoint": {}}):
            with self.subTest(change=change):
                data = json.loads(original)
                data.update(change)
                self.membership_record.write_text(json.dumps(data))
                result = self.run_cli("--preflight")
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.record.exists())
        self.membership_record.write_bytes(original)
        data = json.loads(original)
        for path in (
            self.membership_record.parent / "nginx.conf",
            self.membership_record.parent / "after.compose.json",
            self.membership_record.parent / "configuration.backup",
            Path(data["runtime"]) / "bin" / membership.SOURCE.name,
            self.portal_record,
        ):
            with self.subTest(path=path.name):
                content = path.read_bytes()
                path.write_bytes(content + b"\nchanged\n")
                self.assertNotEqual(self.run_cli("--preflight").returncode, 0)
                path.write_bytes(content)

    def test_membership_container_environment_and_mount_drift_block_release(self):
        state_path = self.root / "fake-state.json"
        original = state_path.read_bytes()
        for change in ("identity", "environment", "mount"):
            with self.subTest(change=change):
                state = json.loads(original)
                if change == "identity":
                    state["containers"]["henukit-account-portfolio-1"]["Id"] = "foreign"
                elif change == "environment":
                    state["containers"]["henukit-account-portfolio-1"]["Config"]["Env"] = ["SECRET=wrong"]
                else:
                    state["containers"]["henukit-nginx-1"]["Mounts"][0]["Source"] = "/foreign"
                state_path.write_text(json.dumps(state))
                result = self.run_cli("--execute")
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.record.exists())
                self.assertEqual(self.marker.resolve(), self.membership_record.parent)
        state_path.write_bytes(original)

    def test_consecutive_portal_release_keeps_the_same_membership_binding(self):
        self.assertEqual(self.run_cli("--execute").returncode, 0)
        retained_record = self.record.parent.with_name("e" * 40)
        self.record.parent.rename(retained_record)
        runtime = self.root / "retained-portal-after-membership"
        shutil.copytree(self.runtime, runtime)
        (runtime / "RELEASE_SHA").write_text("e" * 40 + "\n")
        record = retained_record / "receipt.json"
        data = json.loads(record.read_text())
        data.update(sha="e" * 40, runtime=str(runtime))
        record.write_text(json.dumps(data))
        self.marker.unlink()
        self.marker.symlink_to(retained_record)
        state_path = self.root / "fake-state.json"
        state = json.loads(state_path.read_text())
        item = state["containers"]["henukit-portal-1"]
        item["Config"]["Image"] = "henukit-portal:" + "e" * 40
        state["images"][item["Config"]["Image"]] = item["Image"]
        state_path.write_text(json.dumps(state))
        saved = record.read_bytes(), self.membership_record.read_bytes()
        result = self.run_cli("--execute")
        self.assertEqual(result.returncode, 0, result.stderr)
        current = json.loads(self.record.read_text())
        self.assertEqual(current["predecessor_record"], str(record))
        self.assertEqual(current["membership_checkpoint"], data["membership_checkpoint"])
        self.assertEqual(saved, (record.read_bytes(), self.membership_record.read_bytes()))

    def test_active_portal_cannot_report_success_with_an_unhealthy_membership_owner(self):
        self.assertEqual(self.run_cli("--execute").returncode, 0)
        state_path = self.root / "fake-state.json"
        state = json.loads(state_path.read_text())
        state["containers"]["henukit-account-portfolio-1"]["State"]["Health"] = {"Status": "unhealthy"}
        state_path.write_text(json.dumps(state))
        result = self.run_cli("--execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("membership checkpoint service unhealthy", result.stderr)


for _name in dir(membership.MembershipCLI):
    if _name.startswith("test_"):
        setattr(PortalMembershipCLI, _name, None)


class PortalMembershipBoundaryCLI(PortalMembershipCLI):
    def portal_predecessor(self, receipt_format="henukit-portal-successor-v1", sha="d" * 40):
        for index in range(1, 63):
            portal.PortalCLI.portal_predecessor(self, "henukit-portal-successor-v2", f"{index:040x}")
        return portal.PortalCLI.portal_predecessor(self, receipt_format, sha)

    def test_first_membership_transition_counts_checkpoint_in_64_receipt_limit(self):
        (self.root / "calls.jsonl").unlink()
        result = self.run_cli("--preflight")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("exceeds 64 receipts", result.stderr)
        self.assertFalse(self.record.exists())
        self.assert_no_replacement()


for _name in dir(PortalMembershipCLI):
    if _name.startswith("test_"):
        setattr(PortalMembershipBoundaryCLI, _name, None)
