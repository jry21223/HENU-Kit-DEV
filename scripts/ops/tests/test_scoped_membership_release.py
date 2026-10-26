"""Membership deployment through the real CLI with command-boundary fixtures."""

import hashlib
import io
import json
from pathlib import Path
import shutil
import tarfile

import test_scoped_food_release as food
import test_scoped_portal_release as portal

SOURCE = food.SOURCE.with_name("deploy-henukit-account-membership.py")


class MembershipCLI(portal.PortalCLI):
    def setUp(self):
        super().setUp()
        shutil.copy(SOURCE, self.runtime / "bin" / SOURCE.name)
        for root in (self.runtime, self.root / "opt/henukit-releases" / food.OLD):
            (root / "migrations/account-portfolio").mkdir()
            (root / "migrations/account-portfolio/001.up.sql").write_text(
                "same owner schema"
            )
            (root / "infra/nginx").mkdir(parents=True)
        candidate = self.runtime / "infra/nginx/henukit.conf.example"
        candidate.write_text(
            "  location = /api/v1/bot/membership-entitlement {\n    proxy_pass http://account-portfolio:8097;\n  }\n"
        )
        retained = (
            self.root
            / "opt/henukit-releases"
            / food.OLD
            / "infra/nginx/henukit.conf.example"
        )
        retained.write_text("server {\n  # EasyPay callback\n}\n")
        state_path = self.root / "fake-state.json"
        state = json.loads(state_path.read_text())
        state["containers"]["henukit-nginx-1"]["Mounts"] = [
            {
                "Type": "bind",
                "Source": str(retained),
                "Destination": "/etc/nginx/conf.d/default.conf",
                "RW": False,
            }
        ]
        state["containers"]["henukit-account-portfolio-1"]["Config"]["Env"].append(
            "ACCOUNT_PORTFOLIO_DATABASE_URL=postgres://henukit:password@postgres:5432/account_portfolio?sslmode=disable"
        )
        config = json.dumps(
            {"os": "linux", "architecture": "amd64", "service": "account-portfolio"}
        ).encode()
        state["images"]["henukit-account-portfolio:" + food.NEW] = (
            "sha256:" + hashlib.sha256(config).hexdigest()
        )
        for container in state["containers"].values():
            container["Config"]["Env"] = [
                entry.replace("SECRET=unchanged", "SECRET=unchanged$literal")
                for entry in container["Config"]["Env"]
            ]
        state_path.write_text(json.dumps(state))
        archive = self.artifacts / (
            "henukit-account-portfolio-" + food.NEW + ".docker.tar.gz"
        )
        with tarfile.open(archive, "w:gz") as bundle:
            for name, raw in [
                ("config.json", config),
                (
                    "manifest.json",
                    json.dumps(
                        [
                            {
                                "Config": "config.json",
                                "RepoTags": ["henukit-account-portfolio:" + food.NEW],
                                "Layers": [],
                            }
                        ]
                    ).encode(),
                ),
            ]:
                entry = tarfile.TarInfo(name)
                entry.size = len(raw)
                bundle.addfile(entry, io.BytesIO(raw))
        Path(str(archive) + ".sha256").write_text(
            self.digest(archive) + "  " + archive.name + "\n"
        )
        fake = food.FAKE.replace("unchanged", "unchanged$literal").replace(
            "['food','portal-gateway','portal']",
            "['food','portal-gateway','portal','account-portfolio','nginx']",
        )
        fake = fake.replace(
            "assert sorted(selected)==['food','portal','portal-gateway'], selected",
            "assert selected==['account-portfolio','nginx'], selected",
        )
        marker = "   if os.environ.get('BAD_CONFIG')"
        insertion = """   services['account-portfolio']['environment']['ACCOUNT_PORTFOLIO_DATABASE_URL']='postgres://henukit:password@postgres:5432/account_portfolio?sslmode=disable'
   services['nginx']['volumes']=[{'type':'bind','source':str(root/'opt/henukit-releases'/('a'*40)/'infra/nginx/henukit.conf.example'),'target':'/etc/nginx/conf.d/default.conf','read_only':True}]
   if sha=='b'*40:
    services['account-portfolio']['environment'].update(ACCOUNT_PORTFOLIO_BOT_CLIENT_ID='bot',ACCOUNT_PORTFOLIO_BOT_KEY_ID='bot-key',ACCOUNT_PORTFOLIO_BOT_SECRET='independent-secret-not-logged-32-bytes')
   if os.environ.get('MISSING_BOT') and sha=='b'*40:services['account-portfolio']['environment'].pop('ACCOUNT_PORTFOLIO_BOT_SECRET',None)
   if os.environ.get('CHANGE_OWNER') and sha=='b'*40:services['account-portfolio']['environment']['SECRET']='changed'
"""
        fake = fake.replace(marker, insertion + marker)
        fake = fake.replace(
            " if '/posts/11111111-' in url:",
            " if '/api/v1/bot/membership-entitlement' in url:code=401 if state.get('deployed') else 404\n if '/posts/11111111-' in url:",
        )
        # The fixed private Compose snapshots contain fully rendered image tags.
        fake = fake.replace(
            "sha=os.environ['RELEASE_SHA'];state['deployed']=sha=='b'*40",
            "config=json.loads(pathlib.Path(args[args.index('-f')+1]).read_text());sha=config['services']['account-portfolio']['image'].split(':')[-1];state['deployed']=sha=='b'*40",
        )
        fake = fake.replace(
            "tag='henukit-'+service+':'+sha;c['Id']",
            "tag=config['services'][service]['image'];c['Id']",
        )
        fake = fake.replace(
            "def save():",
            "def literal(v):\n if isinstance(v,str):return v.replace('$$','$')\n if isinstance(v,list):return [literal(x) for x in v]\n if isinstance(v,dict):return {k:literal(x) for k,x in v.items()}\n return v\ndef save():",
        )
        fake = fake.replace(
            " elif 'config' in args:",
            " elif 'config' in args and args[args.index('-f')+1].endswith('.compose.json'):out(literal(json.loads(pathlib.Path(args[args.index('-f')+1]).read_text())))\n  elif 'config' in args:",
        )
        fake = fake.replace(
            "config=json.loads(pathlib.Path(args[args.index('-f')+1]).read_text())",
            "config=literal(json.loads(pathlib.Path(args[args.index('-f')+1]).read_text()))",
        )
        fake = fake.replace(
            "c['Config']['Env']=c['Config']['Env']",
            "c['Config']['Env']=[k+'='+str(v) for k,v in config['services'][service].get('environment',{}).items()];c['Mounts']=[{'Type':v['type'],'Source':v['source'],'Destination':v['target'],'RW':not v.get('read_only',False)} for v in config['services'][service].get('volumes',[])]",
        )
        fake = fake.replace(
            "   if os.environ.get('CHANGE_UNSELECTED')",
            "   if os.environ.get('WRONG_MOUNT') and sha=='b'*40:state['containers']['henukit-nginx-1']['Mounts'][0]['Source']='/wrong'\n   if os.environ.get('WRONG_SECRET') and sha=='b'*40:state['containers']['henukit-account-portfolio-1']['Config']['Env']=['ACCOUNT_PORTFOLIO_BOT_SECRET=wrong']\n   if os.environ.get('CHANGE_UNSELECTED')",
        )
        fake = fake.replace(
            "  out([{'verified':True}])",
            "  if 'account-portfolio-' in args[2]:\n   expected=json.loads(pathlib.Path(args[args.index('--bundle')+1]).read_text())['sha256']\n   if hashlib.sha256(pathlib.Path(args[2]).read_bytes()).hexdigest()!=expected:sys.exit(2)\n  out([{'verified':True}])",
        )
        for name in ("docker", "gh", "systemctl", "curl"):
            (self.root / "bin" / name).write_text(fake)
        proof = self.proof / (
            "henukit-membership-image-" + food.NEW + ".attestation.json"
        )
        proof.write_text(json.dumps({"sha256": self.digest(archive)}))
        self.repack()

    def run_cli(self, mode, extra=None):
        import subprocess

        return subprocess.run(
            [
                "python3",
                str(self.runtime / "bin" / SOURCE.name),
                "--sha",
                food.NEW,
                "--run-id",
                "123",
                "--proof-dir",
                str(self.proof),
                "--test-root",
                str(self.root),
                "--acknowledge-getwork-deferred",
                mode,
            ],
            env={**self.env, **(extra or {})},
            capture_output=True,
            text=True,
        )

    @property
    def record(self):
        return (
            self.root
            / "var/lib/henukit-scoped-releases/account-membership"
            / food.NEW
            / "receipt.json"
        )

    def test_portal_predecessor_keeps_verified_rollback_checkpoint(self):
        predecessor = self.portal_predecessor()
        saved = predecessor.read_bytes()
        self.repack()
        self.assertEqual(self.run_cli("--execute").returncode, 0)
        result = self.run_cli("--rollback")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.marker.resolve(), self.record.parent)
        self.assertEqual(predecessor.read_bytes(), saved)
        self.assertEqual(self.run_cli("--rollback").returncode, 0)
        state_path = self.root / "fake-state.json"
        state = json.loads(state_path.read_text())
        state["containers"]["henukit-nginx-1"]["Id"] = "unreviewed-replacement"
        state_path.write_text(json.dumps(state))
        result = self.run_cli("--rollback")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("checkpoint containers changed", result.stderr)

    def test_unhealthy_owned_candidate_can_roll_back(self):
        self.assertEqual(self.run_cli("--execute").returncode, 0)
        state_path = self.root / "fake-state.json"
        state = json.loads(state_path.read_text())
        owner = state["containers"]["henukit-account-portfolio-1"]
        owner["State"]["Running"] = False
        owner["State"]["Status"] = "exited"
        owner["RestartCount"] += 1
        state_path.write_text(json.dumps(state))
        result = self.run_cli("--rollback")
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_replaced_candidate_cannot_be_overwritten_by_rollback(self):
        self.assertEqual(self.run_cli("--execute").returncode, 0)
        state_path = self.root / "fake-state.json"
        state = json.loads(state_path.read_text())
        state["containers"]["henukit-account-portfolio-1"]["Id"] = "third-party"
        state_path.write_text(json.dumps(state))
        result = self.run_cli("--rollback")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("checkpoint ownership changed", result.stderr)

    def test_substituted_image_and_matching_unsigned_checksum_are_rejected(self):
        archive = self.artifacts / (
            "henukit-account-portfolio-" + food.NEW + ".docker.tar.gz"
        )
        with archive.open("ab") as stream:
            stream.write(b"changed")
        Path(str(archive) + ".sha256").write_text(
            self.digest(archive) + "  " + archive.name + "\n"
        )
        result = self.run_cli("--preflight")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.record.exists())

    def assert_bad_configuration_rolls_back(self, variable):
        result = self.run_cli("--execute", {variable: "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(json.loads(self.record.read_text())["status"], "rolled_back")
        self.assertEqual(self.marker.resolve(), self.record.parent)

    def test_wrong_mount_rolls_back(self):
        self.assert_bad_configuration_rolls_back("WRONG_MOUNT")

    def test_wrong_secret_rolls_back(self):
        self.assert_bad_configuration_rolls_back("WRONG_SECRET")

    def test_membership_preflight_does_not_mutate_current_cohort(self):
        before = self.previous_receipt.read_bytes(), self.envfile.read_bytes()
        result = self.run_cli("--preflight")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            before, (self.previous_receipt.read_bytes(), self.envfile.read_bytes())
        )
        self.assertEqual(self.marker.resolve(), self.previous)
        self.assertFalse(self.record.exists())

    def test_membership_rejects_missing_credential_and_unrelated_owner_changes(self):
        for variable in ("MISSING_BOT", "CHANGE_OWNER"):
            result = self.run_cli("--preflight", {variable: "1"})
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(self.record.exists())

    def test_membership_execute_and_rollback_preserve_portal_cohort(self):
        before = json.loads((self.root / "fake-state.json").read_text())["containers"]
        result = self.run_cli("--execute")
        self.assertEqual(result.returncode, 0, result.stderr)
        receipt = json.loads(self.record.read_text())
        self.assertEqual(receipt["status"], "active")
        after = json.loads((self.root / "fake-state.json").read_text())["containers"]
        for name, container in before.items():
            if name not in ("henukit-account-portfolio-1", "henukit-nginx-1"):
                self.assertEqual(container, after[name])
        result = self.run_cli("--rollback")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.marker.resolve(), self.record.parent)


# Portal predecessor tests are exercised by their own module. Only the
# membership journeys above belong to this derived command fixture.
for _name in list(vars(portal.PortalCLI)):
    if _name.startswith("test_"):
        setattr(MembershipCLI, _name, None)
