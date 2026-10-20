import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import tarfile
import io
import unittest

SOURCE = Path(__file__).resolve().parents[1] / "deploy-henukit-food-scoped.py"
OLD = "a" * 40
NEW = "b" * 40
SELECTED = ["food", "portal-gateway", "portal"]

FAKE = r"""#!/usr/bin/env python3
import hashlib,json,os,pathlib,sys
root=pathlib.Path(os.environ['FIXTURE']); path=root/'fake-state.json'; state=json.loads(path.read_text()); args=sys.argv[1:]; name=pathlib.Path(sys.argv[0]).name
with (root/'calls.jsonl').open('a') as f:f.write(json.dumps([name,args])+chr(10))
def save():path.write_text(json.dumps(state))
def out(v):print(json.dumps(v))
if name=='gh':
 if args[:2]==['run','list']:out([{'databaseId':123,'attempt':1}])
 elif args[:2]==['run','view']:out({'headSha':'b'*40,'headBranch':'main','event':'push','status':'completed','conclusion':'success','workflowName':'Build HENU Kit release artifacts','attempt':1})
 elif args[0]=='api':print(('c' if os.environ.get('STALE_SOURCE') else 'b')*40)
 elif args[:2]==['attestation','verify']:
  if os.environ.get('BAD_PROVENANCE'):sys.exit(2)
  out([{'verified':True}])
 else:sys.exit(92)
elif name=='systemctl':
 if args[0]=='is-active':
  request=root/'var/lib/henukit-actions-watch/quiesce.request'
  if state.get('watcher') and request.exists():
   ack=root/'var/lib/henukit-actions-watch/quiesced';ack.write_text(request.read_text());ack.chmod(0o600);state['watcher']=False;save()
  sys.exit(0 if state.get('watcher') else 3)
 elif args[0]=='show':print('123')
 elif args[0]=='start':state['watcher']=True;save()
 else:sys.exit(0)
elif name=='curl':
 url=args[-1]; code=200
 if '/posts/11111111-' in url:code=401 if state.get('deployed') else 404
 if '/quiz/' in url or '/study-api/' in url:code=404
 if os.environ.get('FAIL_SMOKE') and state.get('deployed'):code=503
 output=args[args.index('--output')+1];pathlib.Path(output).write_text('{}');print(code,end='')
elif name=='docker':
 if args[:2]==['ps','-aq']:print('\n'.join(state['containers']))
 elif args[0]=='inspect':out([state['containers'][n] for n in args[1:]])
 elif args[:2]==['image','inspect']:
  tag=args[2];out([{'Id':state['images'][tag],'Architecture':'amd64','Os':'linux'}])
 elif args[:1]==['load']:print('loaded')
 elif args[0]=='compose':
  if 'config' in args and '--hash' in args:
   service=args[-1];print(service+' config-'+service)
  elif 'config' in args:
   sha=os.environ['RELEASE_SHA'];services={}
   for service in ['food','portal-gateway','portal']:
    services[service]={'image':'henukit-'+service+':'+sha,'environment':{'SECRET':'unchanged'}}
   services['food']['environment']['FOOD_DATABASE_URL']=os.environ.get('FOOD_TEST_URL','postgres://henukit:password@postgres:5432/food?sslmode=disable')
   if os.environ.get('BAD_CONFIG') and sha=='b'*40:services['food']['environment']['SECRET']='changed'
   out({'services':services})
  elif 'up' in args:
   selected=args[args.index('--no-deps')+1:]
   assert sorted(selected)==['food','portal','portal-gateway'], selected
   sha=os.environ['RELEASE_SHA'];state['deployed']=sha=='b'*40
   for service in selected:
    n='henukit-'+service+'-1';c=state['containers'][n];tag='henukit-'+service+':'+sha;c['Id']=service+'-'+sha;c['Image']=state['images'][tag];c['Config']['Image']=tag;c['Config']['Env']=c['Config']['Env']
   if os.environ.get('CHANGE_UNSELECTED') and sha=='b'*40:state['containers']['henukit-career-mcp-1']['Id']='changed'
   save()
  else:sys.exit(93)
 elif args[0]=='exec':
  if any('pg_dump' in a for a in args):
   if os.environ.get('FAIL_BACKUP'):sys.exit(3)
   print('database archive')
  elif any('pg_restore' in a for a in args):sys.stdin.buffer.read()
  elif any('psql' in a for a in args):print('1')
 else:sys.exit(94)
else:sys.exit(95)
"""


class ReleaseCLI(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="henukit-scoped-")
        self.root = Path(self.tmp.name).resolve()
        self.addCleanup(self.tmp.cleanup)
        self.runtime = self.root / "opt/henukit-releases" / NEW
        self.runtime.mkdir(parents=True)
        (self.runtime / "bin").mkdir()
        if SOURCE.exists():
            shutil.copy(SOURCE, self.runtime / "bin" / SOURCE.name)
        installed = self.root / "usr/local/sbin"
        installed.mkdir(parents=True)
        for source, name in [
            ("watch-henukit-actions.sh", "watch-henukit-actions"),
            ("activate-henukit-release.sh", "activate-henukit-release"),
        ]:
            shutil.copy(SOURCE.parent / source, self.runtime / "bin" / source)
            shutil.copy(SOURCE.parent / source, installed / name)
            (installed / name).chmod(0o755)
        for sha in [OLD, NEW]:
            d = self.root / "opt/henukit-releases" / sha
            d.mkdir(exist_ok=True)
            (d / "RELEASE_SHA").write_text(sha + "\n")
            (d / "docker-compose.henukit.release.yml").write_text("services: {}\n")
            (d / "migrations/food").mkdir(parents=True)
            (d / "migrations/food/001.up.sql").write_text("existing schema")
        home = self.root / "opt/henukit"
        home.mkdir()
        (home / "current").symlink_to(self.root / "opt/henukit-releases" / OLD)
        (self.root / "opt/henukit-current").symlink_to(self.root / "opt/henukit-releases" / OLD)
        self.envfile = home / ".env.henukit"
        self.envfile.write_text("SECRET=not-printed\n")
        self.envfile.chmod(0o600)
        state = self.root / "var/lib/henukit-actions-watch"
        state.mkdir(parents=True)
        (state / "approvals").mkdir()
        (state / "rollback-contracts/pending").mkdir(parents=True)
        (state / "last-activated-sha").write_text(OLD + "\n")
        token = self.root / "etc/henukit/github-actions-read.token"
        token.parent.mkdir(parents=True)
        token.write_text("existing-test-token")
        token.chmod(0o600)
        self.proof = self.root / "proof"
        self.proof.mkdir()
        self.artifacts = self.root / "opt/henukit-staging" / NEW
        self.artifacts.mkdir(parents=True)
        images = {}
        for service in SELECTED:
            config = json.dumps({"os": "linux", "architecture": "amd64", "service": service}).encode()
            images["henukit-" + service + ":" + NEW] = "sha256:" + hashlib.sha256(config).hexdigest()
            f = self.artifacts / ("henukit-" + service + "-" + NEW + ".docker.tar.gz")
            with tarfile.open(f, "w:gz") as archive:
                for name, data in [
                    ("config.json", config),
                    (
                        "manifest.json",
                        json.dumps(
                            [
                                {
                                    "Config": "config.json",
                                    "RepoTags": ["henukit-" + service + ":" + NEW],
                                    "Layers": [],
                                }
                            ]
                        ).encode(),
                    ),
                ]:
                    item = tarfile.TarInfo(name)
                    item.size = len(data)
                    archive.addfile(item, io.BytesIO(data))
            Path(str(f) + ".sha256").write_text(
                hashlib.sha256(f.read_bytes()).hexdigest() + "  " + f.name + "\n"
            )
        for name in ["trusted_root.jsonl", "henukit-getwork-actions-" + NEW + ".attestation.json"]:
            (self.proof / name).write_text("{}")
        f = self.artifacts / ("henukit-runtime-" + NEW + ".tar.gz")
        with tarfile.open(f, "w:gz") as archive:
            if SOURCE.exists():
                archive.add(self.runtime / "bin" / SOURCE.name, arcname="./bin/" + SOURCE.name)
            for name in [
                "RELEASE_SHA",
                "docker-compose.henukit.release.yml",
                "migrations/food/001.up.sql",
                "bin/watch-henukit-actions.sh",
                "bin/activate-henukit-release.sh",
            ]:
                archive.add(self.runtime / name, arcname="./" + name)
        Path(str(f) + ".sha256").write_text(hashlib.sha256(f.read_bytes()).hexdigest() + "  " + f.name + "\n")
        (self.proof / ("henukit-getwork-actions-" + NEW + ".manifest")).write_text(
            "artifact_sha256=" + hashlib.sha256(f.read_bytes()).hexdigest() + "  " + f.name + "\n"
        )
        containers = {}
        for s in SELECTED + [
            "postgres",
            "redis",
            "nginx",
            "library",
            "account-portfolio",
            "portal-summary",
            "platform-core",
            "career-opportunities",
            "career-mcp",
            "getwork-mcp-relay",
        ]:
            tag = "henukit-" + s + ":" + OLD
            ignored = s.startswith("career") or s.startswith("getwork")
            containers["henukit-" + s + "-1"] = {
                "Id": "original-" + s,
                "Name": "/henukit-" + s + "-1",
                "Image": hashlib.sha256(tag.encode()).hexdigest(),
                "Config": {
                    "Image": tag,
                    "Env": ["SECRET=unchanged"],
                    "Labels": {"com.docker.compose.config-hash": "config-" + s},
                },
                "State": {
                    "Status": "exited" if ignored else "running",
                    "Running": not ignored,
                    "Health": {"Status": "unhealthy" if ignored else "healthy"},
                },
                "RestartCount": 0,
            }
        containers["henukit-portal-summary-1"]["Config"]["Env"] = [
            "PORTAL_VERSION=" + OLD,
            "PORTAL_DEPLOYED_AT=2026-10-12T00:00:00Z",
        ]
        containers["henukit-food-1"]["Config"]["Env"].append(
            "FOOD_DATABASE_URL=postgres://henukit:password@postgres:5432/food?sslmode=disable"
        )
        containers["henukit-postgres-1"]["Config"]["Env"] = [
            "POSTGRES_USER=henukit",
            "POSTGRES_PASSWORD=password",
        ]
        images.update({c["Config"]["Image"]: c["Image"] for c in containers.values()})
        (self.root / "fake-state.json").write_text(json.dumps({"containers": containers, "images": images}))
        (self.root / "bin").mkdir()
        for name in ["docker", "gh", "systemctl", "curl"]:
            f = self.root / "bin" / name
            f.write_text(FAKE)
            f.chmod(0o755)
        self.env = {
            **os.environ,
            "PATH": str(self.root / "bin") + ":" + os.environ["PATH"],
            "FIXTURE": str(self.root),
        }

    def run_cli(self, mode, extra=None):
        cmd = [
            "python3",
            str(self.runtime / "bin" / SOURCE.name),
            "--sha",
            NEW,
            "--run-id",
            "123",
            "--proof-dir",
            str(self.proof),
            "--test-root",
            str(self.root),
            "--acknowledge-getwork-deferred",
            mode,
        ]
        return subprocess.run(cmd, env={**self.env, **(extra or {})}, capture_output=True, text=True)

    def calls(self):
        p = self.root / "calls.jsonl"
        return [json.loads(x) for x in p.read_text().splitlines()] if p.exists() else []

    def test_execute_replaces_only_fixed_scope_and_records_deferred_acceptance(self):
        before = json.loads((self.root / "fake-state.json").read_text())["containers"]
        r = self.run_cli("--execute")
        self.assertEqual(r.returncode, 0, r.stderr)
        after = json.loads((self.root / "fake-state.json").read_text())["containers"]
        for name, item in before.items():
            if name not in ["henukit-" + s + "-1" for s in SELECTED]:
                self.assertEqual(after[name], item)
        marker = self.root / "var/lib/henukit-actions-watch/scoped-food-release"
        self.assertTrue(marker.is_symlink())
        receipt = json.loads((marker / "receipt.json").read_text())
        self.assertEqual(receipt["status"], "active")
        self.assertEqual(receipt["getwork_acceptance"], "deferred")
        self.assertFalse(receipt["whole_application_release"])
        self.assertEqual((self.root / "opt/henukit/current").resolve().name, OLD)
        self.assertEqual(
            (self.root / "var/lib/henukit-actions-watch/last-activated-sha").read_text().strip(), OLD
        )

    def test_explicit_rollback_recovers_interrupted_attempt_and_orphaned_quiescence(self):
        r = self.run_cli("--execute")
        self.assertEqual(r.returncode, 0, r.stderr)
        marker = self.root / "var/lib/henukit-actions-watch/scoped-food-release"
        receipt = (marker / "receipt.json").resolve()
        data = json.loads(receipt.read_text())
        data["status"] = "applying"
        receipt.write_text(json.dumps(data))
        receipt.chmod(0o600)
        value = NEW + " 123 " + ("c" * 32) + "\n"
        for name in ["quiesce.request", "quiesced"]:
            path = self.root / "var/lib/henukit-actions-watch" / name
            path.write_text(value)
            path.chmod(0o600)
        r = self.run_cli("--rollback")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertFalse(marker.is_symlink())
        self.assertEqual(json.loads(receipt.read_text())["status"], "rolled_back")
        state = json.loads((self.root / "fake-state.json").read_text())
        for service in SELECTED:
            self.assertEqual(
                state["containers"]["henukit-" + service + "-1"]["Config"]["Image"],
                "henukit-" + service + ":" + OLD,
            )

    def test_failed_acceptance_rolls_back_only_selected_services(self):
        r = self.run_cli("--execute", {"FAIL_SMOKE": "1"})
        self.assertNotEqual(r.returncode, 0)
        record = self.root / "var/lib/henukit-scoped-releases/food-author-edit" / NEW / "receipt.json"
        self.assertEqual(json.loads(record.read_text())["status"], "rolled_back")
        state = json.loads((self.root / "fake-state.json").read_text())
        for service in SELECTED:
            self.assertEqual(
                state["containers"]["henukit-" + service + "-1"]["Config"]["Image"],
                "henukit-" + service + ":" + OLD,
            )
        self.assertFalse((self.root / "var/lib/henukit-actions-watch/scoped-food-release").is_symlink())

    def test_backup_failure_prevents_image_loading_and_container_mutation(self):
        r = self.run_cli("--execute", {"FAIL_BACKUP": "1"})
        self.assertNotEqual(r.returncode, 0)
        self.assertFalse(any(c[0] == "docker" and ("up" in c[1] or "load" in c[1]) for c in self.calls()))

    def test_config_drift_prevents_mutation(self):
        r = self.run_cli("--execute", {"BAD_CONFIG": "1"})
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("incompatible", r.stderr)
        self.assertFalse(
            any(c[0] == "docker" and ("up" in c[1] or "load" in c[1] or "exec" in c[1]) for c in self.calls())
        )

    def test_archive_tampering_prevents_mutation(self):
        (self.artifacts / ("henukit-food-" + NEW + ".docker.tar.gz")).write_bytes(b"tampered")
        r = self.run_cli("--execute")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("checksum mismatch", r.stderr)
        self.assertFalse(
            any(c[0] == "docker" and ("up" in c[1] or "load" in c[1] or "exec" in c[1]) for c in self.calls())
        )

    def test_repeat_execute_does_not_replace_containers_again(self):
        r = self.run_cli("--execute")
        self.assertEqual(r.returncode, 0, r.stderr)
        r = self.run_cli("--execute")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(r.stdout)["status"], "already_active")
        self.assertEqual(sum(c[0] == "docker" and "up" in c[1] for c in self.calls()), 1)

    def test_unselected_change_is_not_repaired_or_reported_healthy(self):
        r = self.run_cli("--execute", {"CHANGE_UNSELECTED": "1"})
        self.assertNotEqual(r.returncode, 0)
        record = self.root / "var/lib/henukit-scoped-releases/food-author-edit" / NEW / "receipt.json"
        self.assertEqual(json.loads(record.read_text())["status"], "rollback_failed")
        state = json.loads((self.root / "fake-state.json").read_text())
        self.assertEqual(state["containers"]["henukit-career-mcp-1"]["Id"], "changed")
        for service in SELECTED:
            self.assertEqual(
                state["containers"]["henukit-" + service + "-1"]["Config"]["Image"],
                "henukit-" + service + ":" + OLD,
            )

    def test_active_watcher_uses_safe_quiescence_and_is_restored(self):
        file = self.root / "fake-state.json"
        state = json.loads(file.read_text())
        state["watcher"] = True
        file.write_text(json.dumps(state))
        instance = self.root / "var/lib/henukit-actions-watch/watcher.instance"
        instance.write_text("123\n")
        instance.chmod(0o600)
        r = self.run_cli("--execute")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertTrue(json.loads(file.read_text())["watcher"])
        self.assertFalse((instance.parent / "quiesce.request").exists())

    def test_stale_source_and_failed_provenance_prevent_all_writes(self):
        for override in [{"STALE_SOURCE": "1"}, {"BAD_PROVENANCE": "1"}]:
            result = self.run_cli("--execute", override)
            self.assertNotEqual(result.returncode, 0)
        self.assertFalse(
            any(c[0] == "docker" and ("up" in c[1] or "load" in c[1] or "exec" in c[1]) for c in self.calls())
        )

    def test_public_readable_environment_is_rejected(self):
        self.envfile.chmod(0o644)
        result = self.run_cli("--execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("private file mode", result.stderr)

    def test_rollback_refuses_a_changed_full_alias(self):
        result = self.run_cli("--execute")
        self.assertEqual(result.returncode, 0, result.stderr)
        legacy = self.root / "opt/henukit-current"
        legacy.unlink()
        legacy.symlink_to(self.runtime)
        count = sum(c[0] == "docker" and "up" in c[1] for c in self.calls())
        result = self.run_cli("--rollback")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(sum(c[0] == "docker" and "up" in c[1] for c in self.calls()), count)

    def test_stale_full_release_guard_prevents_all_mutations(self):
        path = self.root / "usr/local/sbin/watch-henukit-actions"
        path.write_text(path.read_text() + "\n# stale guard\n")
        result = self.run_cli("--execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("installed full-release guard", result.stderr)
        self.assertFalse(
            any(c[0] == "docker" and ("up" in c[1] or "load" in c[1] or "exec" in c[1]) for c in self.calls())
        )

    def test_terminal_rollback_retry_reconciles_marker_and_watcher_without_replacement(self):
        self.assertEqual(self.run_cli("--execute").returncode, 0)
        self.assertEqual(self.run_cli("--rollback").returncode, 0)
        watch = self.root / "var/lib/henukit-actions-watch"
        marker = watch / "scoped-food-release"
        marker.symlink_to(self.root / "var/lib/henukit-scoped-releases/food-author-edit" / NEW)
        for name in ("quiesce.request", "quiesced"):
            file = watch / name
            file.write_text(NEW + " 123 " + "c" * 32 + "\n")
            file.chmod(0o600)
        count = sum(c[0] == "docker" and "up" in c[1] for c in self.calls())
        result = self.run_cli("--rollback")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(marker.is_symlink())
        self.assertFalse((watch / "quiesce.request").exists())
        self.assertTrue(json.loads((self.root / "fake-state.json").read_text())["watcher"])
        self.assertEqual(sum(c[0] == "docker" and "up" in c[1] for c in self.calls()), count)

    def test_retagged_rollback_image_prevents_backup_and_replacement(self):
        file = self.root / "fake-state.json"
        state = json.loads(file.read_text())
        state["images"]["henukit-food:" + OLD] = "retagged"
        file.write_text(json.dumps(state))
        result = self.run_cli("--execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("rollback image", result.stderr)
        self.assertFalse(
            any(c[0] == "docker" and ("up" in c[1] or "load" in c[1] or "exec" in c[1]) for c in self.calls())
        )

    def test_running_environment_and_original_compose_hash_must_match_baseline(self):
        file = self.root / "fake-state.json"
        original = json.loads(file.read_text())
        for field in ("Env", "Labels"):
            state = json.loads(json.dumps(original))
            config = state["containers"]["henukit-food-1"]["Config"]
            if field == "Env":
                config["Env"][0] = "SECRET=drift"
            else:
                config["Labels"]["com.docker.compose.config-hash"] = "drift"
            file.write_text(json.dumps(state))
            result = self.run_cli("--preflight")
            self.assertNotEqual(result.returncode, 0, field)
            self.assertIn("running configuration", result.stderr)

    def test_backup_target_is_bound_to_actual_food_connection(self):
        file = self.root / "fake-state.json"
        state = json.loads(file.read_text())
        for url in (
            "postgres://henukit:password@remote:5432/food",
            "postgres://henukit:password@postgres:5432/food?host=remote",
            "postgres://henukit:password@postgres:5432/food?dbname=other",
        ):
            state["containers"]["henukit-food-1"]["Config"]["Env"][-1] = "FOOD_DATABASE_URL=" + url
            file.write_text(json.dumps(state))
            result = self.run_cli("--execute", {"FOOD_TEST_URL": url})
            self.assertNotEqual(result.returncode, 0, url)
            self.assertIn("Food backup target", result.stderr)
        self.assertFalse(any(c[0] == "docker" and "exec" in c[1] for c in self.calls()))

    def test_preflight_is_read_only(self):
        r = self.run_cli("--preflight")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertFalse(
            any(c[0] == "docker" and ("up" in c[1] or "load" in c[1] or "exec" in c[1]) for c in self.calls())
        )
        self.assertFalse((self.root / "var/lib/henukit-actions-watch/scoped-food-release").exists())


if __name__ == "__main__":
    unittest.main()
