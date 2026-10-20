"""Fixed Practice CLI: real subprocess seam with private local command fixtures."""
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import unittest

import test_scoped_food_release as food

SOURCE = food.SOURCE.with_name("deploy-henukit-practice-scoped.py")
SELECTED = ("portal", "portal-gateway", "food", "console-gateway", "quizcraft")
MIGRATION = b"existing QuizCraft migration\n"


class PracticeCLI(unittest.TestCase):
    def setUp(self):
        food.ReleaseCLI.setUp(self)
        shutil.copy(food.SOURCE, self.runtime / "bin" / food.SOURCE.name)
        if SOURCE.exists():
            shutil.copy(SOURCE, self.runtime / "bin" / SOURCE.name)
        state_file = self.root / "fake-state.json"
        state = json.loads(state_file.read_text())
        prototype = state["containers"]["henukit-food-1"]
        for service in ("console-gateway", "quizcraft"):
            item = json.loads(json.dumps(prototype))
            item.update(Id="original-" + service, Name="/henukit-" + service + "-1")
            tag = "henukit-" + service + ":" + food.OLD
            item["Image"] = hashlib.sha256(tag.encode()).hexdigest()
            item["Config"] = {"Image": tag, "Env": ["SECRET=unchanged"], "Labels": {"com.docker.compose.config-hash": "config-" + service}}
            state["containers"][item["Name"][1:]] = item
            state["images"][tag] = item["Image"]
            config = json.dumps({"os": "linux", "architecture": "amd64", "service": service}).encode()
            state["images"]["henukit-" + service + ":" + food.NEW] = "sha256:" + hashlib.sha256(config).hexdigest()
            archive = self.artifacts / ("henukit-" + service + "-" + food.NEW + ".docker.tar.gz")
            with tarfile.open(archive, "w:gz") as stream:
                for name, content in [("config.json", config), ("manifest.json", json.dumps([{"Config": "config.json", "RepoTags": ["henukit-" + service + ":" + food.NEW], "Layers": []}]).encode())]:
                    info = tarfile.TarInfo(name); info.size = len(content); stream.addfile(info, io.BytesIO(content))
            self.checksum(archive)
        quiz = state["containers"]["henukit-quizcraft-1"]
        quiz["Config"]["Env"].append("DATABASE_URL=postgres://quizcraft_v2:password@host.docker.internal:5432/quizcraft_v2?sslmode=disable")
        quiz["Id"] = "1" * 64
        hosts = self.root / "var/lib/docker/containers" / quiz["Id"] / "hosts"
        hosts.parent.mkdir(parents=True)
        hosts.write_text("127.0.0.1 localhost\n172.17.0.1 host.docker.internal\n")
        quiz["HostsPath"] = str(hosts)
        quiz["HostConfig"] = {"ExtraHosts": ["host.docker.internal:host-gateway"]}
        quiz["NetworkSettings"] = {"Networks": {"henukit_default": {"Gateway": "172.18.0.1", "IPAddress": "172.18.0.9"}}}
        state_file.write_text(json.dumps(state))
        migrations = self.runtime / "migrations/quizcraft"
        migrations.mkdir(parents=True)
        (migrations / "000001.up.sql").write_bytes(MIGRATION)
        self.repack()
        fake = food.FAKE.replace("for service in ['food','portal-gateway','portal']:", "for service in ['portal','portal-gateway','food','console-gateway','quizcraft']:")
        fake = fake.replace("services['food']['environment']['FOOD_DATABASE_URL']=os.environ.get('FOOD_TEST_URL','postgres://henukit:password@postgres:5432/food?sslmode=disable')", """services['food']['environment']['FOOD_DATABASE_URL']=os.environ.get('FOOD_TEST_URL','postgres://henukit:password@postgres:5432/food?sslmode=disable')
   services['quizcraft']['environment']['DATABASE_URL']='postgres://quizcraft_v2:password@host.docker.internal:5432/quizcraft_v2?sslmode=disable'
   if sha=='b'*40:
    for service in ['console-gateway','quizcraft']:
     keys=['QUIZCRAFT_CONSOLE_ENABLED','QUIZCRAFT_CONSOLE_CLIENT_ID']+(['QUIZCRAFT_CONSOLE_API_URL','QUIZCRAFT_CONSOLE_KEY_ID','QUIZCRAFT_CONSOLE_SECRET'] if service=='console-gateway' else ['QUIZCRAFT_CONSOLE_KEYS_JSON'])
     services[service]['environment'].update({k:os.environ[k] for k in keys})""")
        fake = fake.replace("services['food']['environment']['SECRET']='changed'", "services['quizcraft']['environment']['SECRET']='changed'")
        fake = fake.replace("['food','portal','portal-gateway']", "['console-gateway','food','portal','portal-gateway','quizcraft']")
        fake = fake.replace("out({'services':services})", """result={'services':services}
   if sha=='b'*40:
    if os.environ.get('BAD_ROOT_CONFIG'):result['networks']={'evil':{}}
    if os.environ.get('BAD_SERVICE_CONFIG'):services['portal']['read_only']=True
    if os.environ.get('BAD_FOOD_CONFIG'):services['food']['read_only']=True
    if os.environ.get('BAD_PORTAL_GATEWAY_CONFIG'):services['portal-gateway']['read_only']=True
    if os.environ.get('BAD_ENV_ADDITION'):services['quizcraft']['environment']['UNREVIEWED']='1'
   out(result)""")
        fake = fake.replace("c['Config']['Env']=c['Config']['Env']", """c['Config']['Env']=[v for v in c['Config']['Env'] if not v.startswith('QUIZCRAFT_CONSOLE_')]
    if sha=='b'*40:
     keys=['QUIZCRAFT_CONSOLE_ENABLED','QUIZCRAFT_CONSOLE_CLIENT_ID']+(['QUIZCRAFT_CONSOLE_API_URL','QUIZCRAFT_CONSOLE_KEY_ID','QUIZCRAFT_CONSOLE_SECRET'] if service=='console-gateway' else ['QUIZCRAFT_CONSOLE_KEYS_JSON']) if service in ['console-gateway','quizcraft'] else []
     c['Config']['Env'] += [k+'='+os.environ[k] for k in keys]""")
        fake = fake.replace("if os.environ.get('CHANGE_UNSELECTED') and sha=='b'*40:", """q=state['containers']['henukit-quizcraft-1'];q['Id']=('2' if sha=='b'*40 else '1')*64
   hosts=root/'var/lib/docker/containers'/q['Id']/'hosts';hosts.parent.mkdir(parents=True,exist_ok=True);hosts.write_text('172.17.0.1 host.docker.internal\\n');q['HostsPath']=str(hosts)
   if os.environ.get('CHANGE_UNSELECTED') and sha=='b'*40:""")
        fake = fake.replace("elif args[0]=='api':print(('c' if os.environ.get('STALE_SOURCE') else 'b')*40)", """elif args[0]=='api':
  if '/contents/' in args[1]:
   import base64
   content=b'existing QuizCraft migration\\n'
   if 'migrations?ref=' in args[1]:out([{'name':'000001.up.sql','path':'products/quizcraft/go-service/db/migrations/000001.up.sql','type':'file','sha':hashlib.sha1(b'blob '+str(len(content)).encode()+b'\\0'+content).hexdigest() if not (os.environ.get('BAD_MIGRATION') and 'ref='+('b'*40) in args[1]) else 'f'*40}])
   else:out({'encoding':'base64','content':base64.b64encode(content).decode(),'sha':hashlib.sha1(b'blob '+str(len(content)).encode()+b'\\0'+content).hexdigest()})
  else:print(('c' if os.environ.get('STALE_SOURCE') else 'b')*40)""")
        fake = fake.replace("if '/posts/11111111-' in url:", "if '/api/v1/console/workshop/catalog' in url:code=401 if state.get('deployed') else 404\n if '/posts/11111111-' in url:")
        fake = fake.replace("if '/posts/11111111-' in url:", "if '/api/v1/quizcraft/workshop/catalog' in url:code=401 if state.get('deployed') else 404\n if '/posts/11111111-' in url:")
        fake = fake.replace("if '/quiz' in url or '/study-api/' in url:code=404", "if '/quiz/' in url or url.endswith('/quiz') or '/study-api/' in url:code=404")
        fake = fake.replace("if os.environ.get('FAIL_BACKUP'):sys.exit(3)\n   print('database archive')", "if os.environ.get('FAIL_FOOD_BACKUP'):sys.exit(3)\n   print('database archive')")
        fake = fake.replace("elif any('pg_restore' in a for a in args):sys.stdin.buffer.read()", "elif any('pg_restore' in a for a in args):\n   sys.stdin.buffer.read()\n   if os.environ.get('FAIL_FOOD_RESTORE'):sys.exit(3)")
        fake = fake.replace("elif any('psql' in a for a in args):print('1')", "elif any('psql' in a for a in args):print('bad' if os.environ.get('BAD_FOOD_COUNTS') else '1')")
        fake = fake.replace("elif name=='docker':", """elif name in ['psql','pg_dump','pg_restore','runuser','createdb','dropdb']:
 if '--version' in args:print(name+' (PostgreSQL) 16.15')
 elif name=='runuser':
  tool=pathlib.Path(args[args.index('--')+1]).name
  if tool=='psql':print(json.dumps({'database':'quizcraft_v2','oid':42,'started':'2026-10-01','port':5432,'version':160015}))
  if tool=='pg_restore' and os.environ.get('FAIL_RESTORE'):sys.exit(3)
  if tool=='psql' and any('quizcraft_banks' in a for a in args):print('') if False else None
 elif name=='psql':
  if '--command' not in args:
   import time
   line=sys.stdin.readline()
   print(json.dumps({'snapshot':'00000003-0000001A-1','counts':{'banks':1,'versions':2,'questions':3,'question_versions':4,'memberships':4,'attempts':5,'sessions':6,'audit':7}}),flush=True)
   sys.stdin.read();sys.exit(0)
  if any('quizcraft_banks' in a for a in args):print(json.dumps({'banks':1,'versions':2,'questions':3,'question_versions':4,'memberships':4,'attempts':5,'sessions':6,'audit':7}))
  else:print(json.dumps({'database':'quizcraft_v2','oid':42,'started':'2026-10-01','port':5432,'version':160015}) if not os.environ.get('BAD_TARGET') else '{}')
 elif name=='pg_dump':
  if os.environ.get('FAIL_BACKUP'):sys.exit(3)
  pathlib.Path(args[args.index('--file')+1]).write_bytes(b'PGDMP fixture archive')
 elif name=='pg_restore':
  if os.environ.get('FAIL_LIST'):sys.exit(3)
  print('archive contents')
elif name=='docker':""")
        # Admin count queries must return the same snapshot facts after restore.
        fake = fake.replace("if tool=='psql':print(json.dumps({'database'", "if tool=='psql' and not any('quizcraft_banks' in a for a in args):print(json.dumps({'database'")
        fake = fake.replace("if tool=='psql' and any('quizcraft_banks' in a for a in args):print('') if False else None", "if tool=='psql' and any('quizcraft_banks' in a for a in args):print(json.dumps({'banks':1,'versions':2,'questions':3,'question_versions':4,'memberships':4,'attempts':9 if os.environ.get('BAD_COUNTS') else 5,'sessions':6,'audit':7}))")
        for name in ("docker", "gh", "systemctl", "curl", "psql", "pg_dump", "pg_restore", "runuser", "createdb", "dropdb"):
            path = self.root / "bin" / name; path.write_text(fake); path.chmod(0o755)

    def checksum(self, path):
        Path(str(path) + ".sha256").write_text(hashlib.sha256(path.read_bytes()).hexdigest() + "  " + path.name + "\n")

    def repack(self):
        archive = self.artifacts / ("henukit-runtime-" + food.NEW + ".tar.gz")
        with tarfile.open(archive, "w:gz") as stream:
            for path in self.runtime.rglob("*"):
                if path.is_file():
                    stream.add(path, arcname="./" + str(path.relative_to(self.runtime)))
        self.checksum(archive)
        (self.proof / ("henukit-getwork-actions-" + food.NEW + ".manifest")).write_text("artifact_sha256=" + hashlib.sha256(archive.read_bytes()).hexdigest() + "  " + archive.name + "\n")

    def run_cli(self, mode, override=None):
        return subprocess.run(["python3", str(self.runtime / "bin" / SOURCE.name), "--sha", food.NEW, "--run-id", "123", "--proof-dir", str(self.proof), "--test-root", str(self.root), "--acknowledge-getwork-deferred", mode], env={**self.env, **(override or {})}, capture_output=True, text=True)

    def calls(self):
        return food.ReleaseCLI.calls(self)

    @property
    def record(self):
        return self.root / "var/lib/henukit-scoped-releases/practice-console" / food.NEW / "receipt.json"

    def test_preflight_is_read_only_and_does_not_create_application_key(self):
        result = self.run_cli("--preflight")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(self.record.parent.exists())
        self.assertFalse(any(name in ("pg_dump", "createdb", "dropdb") and "--version" not in args or name == "runuser" and any(Path(a).name in ("createdb", "dropdb", "pg_restore") for a in args) or name == "docker" and ("load" in args or "up" in args) for name, args in self.calls()))

    def test_execute_uses_only_fixed_scope_and_keeps_private_key_after_backup(self):
        before = json.loads((self.root / "fake-state.json").read_text())["containers"]
        environment = self.envfile.read_bytes()
        result = self.run_cli("--execute")
        self.assertEqual(result.returncode, 0, result.stderr)
        data = json.loads(self.record.read_text())
        self.assertEqual(data["status"], "active")
        self.assertEqual(json.loads(result.stdout)["scope"], ["portal", "portal-gateway", "food", "console-gateway", "quizcraft"])
        self.assertEqual(set(data["images"]), {"portal", "portal-gateway", "food", "console-gateway", "quizcraft"})
        self.assertEqual(data["database_backups"]["quizcraft"]["counts"]["attempts"], 5)
        self.assertEqual(data["database_backups"]["food"]["counts"]["food_posts"], 1)
        for database in ("food", "quizcraft"):
            archive = self.record.parent / (database + ".dump")
            self.assertEqual(data["database_backups"][database]["sha256"], hashlib.sha256(archive.read_bytes()).hexdigest())
            self.assertEqual(data["database_backups"][database]["bytes"], archive.stat().st_size)
        probes = [(args[-1], args[args.index("--request") + 1]) for tool, args in self.calls() if tool == "curl"]
        self.assertIn(("https://henukit.cn/api/v1/food/posts/11111111-1111-4111-8111-111111111111/edit", "GET"), probes)
        self.assertIn(("https://henukit.cn/api/v1/food/posts/11111111-1111-4111-8111-111111111111", "PUT"), probes)
        self.assertEqual(self.envfile.read_bytes(), environment)
        after = json.loads((self.root / "fake-state.json").read_text())["containers"]
        for name in before:
            if name not in {"henukit-" + s + "-1" for s in SELECTED}:
                self.assertEqual(after[name], before[name])
        secret_file = self.record.parent / "quizcraft-console-application.json"
        self.assertEqual(secret_file.stat().st_mode & 0o777, 0o600)
        secret = json.loads(secret_file.read_text())["secret"]
        self.assertNotIn(secret, result.stdout + result.stderr + self.record.read_text())
        self.assertTrue((self.root / "var/lib/henukit-actions-watch/scoped-food-release").is_symlink())

    def test_backup_restore_and_count_failures_prevent_key_and_image_loading(self):
        for flag in ("FAIL_FOOD_BACKUP", "FAIL_FOOD_RESTORE", "BAD_FOOD_COUNTS", "FAIL_BACKUP", "FAIL_LIST", "FAIL_RESTORE", "BAD_COUNTS"):
            with self.subTest(flag=flag):
                case = PracticeCLI(); case.setUp()
                try:
                    result = case.run_cli("--execute", {flag: "1"})
                    self.assertNotEqual(result.returncode, 0, result.stderr)
                    self.assertFalse((case.record.parent / "quizcraft-console-application.json").exists())
                    self.assertFalse(any(tool == "docker" and ("load" in args or "up" in args) for tool, args in case.calls()))
                    if flag.startswith("FAIL_FOOD") or flag == "BAD_FOOD_COUNTS":
                        self.assertFalse(any(tool == "pg_dump" and "--file" in args for tool, args in case.calls()))
                    else:
                        partial = json.loads(case.record.read_text())
                        self.assertEqual(partial["database_backups"]["food"]["counts"], {"food_posts": 1})
                        self.assertNotIn("quizcraft", partial["database_backups"])
                    drops = [args for tool, args in case.calls() if tool == "runuser" and any(Path(a).name == "dropdb" for a in args)]
                    for args in drops:
                        self.assertRegex(args[-1], r"^henukit_practice_verify_b{8}_[0-9a-f]{24}$")
                finally:
                    case.doCleanups()

    def test_configuration_migrations_target_and_runtime_pins_fail_before_mutation(self):
        for flag in ("BAD_CONFIG", "BAD_ROOT_CONFIG", "BAD_SERVICE_CONFIG", "BAD_FOOD_CONFIG", "BAD_PORTAL_GATEWAY_CONFIG", "BAD_ENV_ADDITION", "BAD_MIGRATION", "BAD_TARGET", "STALE_SOURCE", "BAD_PROVENANCE"):
            result = self.run_cli("--preflight", {flag: "1"})
            self.assertNotEqual(result.returncode, 0, flag)
        self.assertFalse(self.record.parent.exists())
        helper = self.runtime / "bin" / SOURCE.name
        helper.write_text(helper.read_text() + "\n# unattested edit\n")
        result = self.run_cli("--execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("scope entry differs", result.stderr)
        self.assertFalse(any(tool == "docker" and ("load" in args or "up" in args) for tool, args in self.calls()))

    def test_food_target_and_schema_gate_remain_mandatory_for_five_service_release(self):
        state_file = self.root / "fake-state.json"
        original = state_file.read_text()
        state = json.loads(original)
        url = "postgres://henukit:password@postgres:5432/other?sslmode=disable"
        state["containers"]["henukit-food-1"]["Config"]["Env"][-1] = "FOOD_DATABASE_URL=" + url
        state_file.write_text(json.dumps(state))
        result = self.run_cli("--preflight", {"FOOD_TEST_URL": url})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Food backup target", result.stderr)
        state_file.write_text(original)
        (self.runtime / "migrations/food/001.up.sql").write_text("changed Food schema")
        self.repack()
        result = self.run_cli("--execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Food schema changes", result.stderr)
        self.assertFalse(any(tool == "docker" and ("load" in args or "up" in args or "exec" in args) for tool, args in self.calls()))

    def test_repeat_execute_and_rollback_verify_without_replacement(self):
        first = self.run_cli("--execute")
        self.assertEqual(first.returncode, 0, first.stderr)
        second = self.run_cli("--execute")
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(json.loads(second.stdout)["status"], "already_active")
        self.assertEqual(sum(tool == "docker" and "up" in args for tool, args in self.calls()), 1)
        rolled = self.run_cli("--rollback")
        self.assertEqual(rolled.returncode, 0, rolled.stderr)
        self.assertEqual(json.loads(self.record.read_text())["status"], "rolled_back")
        repeated = self.run_cli("--rollback")
        self.assertEqual(repeated.returncode, 0, repeated.stderr)
        self.assertEqual(sum(tool == "docker" and "up" in args for tool, args in self.calls()), 2)
        self.assertFalse((self.root / "var/lib/henukit-actions-watch/scoped-food-release").is_symlink())
        self.assertTrue((self.record.parent / "quizcraft-console-application.json").exists())

    def test_failed_smoke_uses_original_configuration_and_only_selected_images(self):
        result = self.run_cli("--execute", {"FAIL_SMOKE": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(json.loads(self.record.read_text())["status"], "rolled_back")
        state = json.loads((self.root / "fake-state.json").read_text())
        for service in SELECTED:
            item = state["containers"]["henukit-" + service + "-1"]
            self.assertTrue(item["Config"]["Image"].endswith(food.OLD))
            self.assertFalse(any(v.startswith("QUIZCRAFT_CONSOLE_") for v in item["Config"]["Env"]))

    def test_interrupted_attempt_requires_explicit_rollback(self):
        result = self.run_cli("--execute")
        self.assertEqual(result.returncode, 0, result.stderr)
        data = json.loads(self.record.read_text()); data["status"] = "applying"
        self.record.write_text(json.dumps(data))
        retry = self.run_cli("--execute")
        self.assertNotEqual(retry.returncode, 0)
        self.assertIn("interrupted", retry.stderr)
        rolled = self.run_cli("--rollback")
        self.assertEqual(rolled.returncode, 0, rolled.stderr)

    def test_rollback_pins_engine_secret_and_compose_without_mutation(self):
        result = self.run_cli("--execute")
        self.assertEqual(result.returncode, 0, result.stderr)
        count = sum(tool == "docker" and "up" in args for tool, args in self.calls())
        for path in (self.runtime / "bin" / food.SOURCE.name, self.record.parent / "quizcraft-console-application.json", self.runtime / "docker-compose.henukit.release.yml"):
            original = path.read_bytes()
            path.write_bytes(original + b"\n")
            result = self.run_cli("--rollback")
            self.assertNotEqual(result.returncode, 0, str(path))
            path.write_bytes(original)
        self.assertEqual(sum(tool == "docker" and "up" in args for tool, args in self.calls()), count)

    def test_unselected_drift_keeps_marker_after_failed_rollback(self):
        result = self.run_cli("--execute", {"CHANGE_UNSELECTED": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(json.loads(self.record.read_text())["status"], "rollback_failed")
        self.assertTrue((self.root / "var/lib/henukit-actions-watch/scoped-food-release").is_symlink())


if __name__ == "__main__":
    unittest.main()
