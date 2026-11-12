"""Core full-release owner: public CLI with bounded OS fixtures."""

import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest

import test_scoped_practice_release as practice
import test_scoped_food_release as food

SOURCE = Path(__file__).resolve().parents[1]
SHA = "a" * 40
CAP = {
    "format": "henukit-data-consumer-capabilities-v1",
    "release_sha": SHA,
    "consumers": {
        s: ["english-training-data-v1"]
        for s in ("quizcraft", "portal-gateway", "portal")
    },
}


class CapabilityCLI(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="hc651-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.runtime = self.root / "releases" / SHA
        (self.runtime / "bin").mkdir(parents=True)
        (self.runtime / "release-gates").mkdir()
        for name in ("henukit-quizcraft-release.py", "henukit-quizcraft-database.py"):
            if (SOURCE / name).exists():
                shutil.copyfile(SOURCE / name, self.runtime / "bin" / name)
        (self.runtime / "release-gates/english-training-capabilities.json").write_text(
            json.dumps(CAP)
        )
        self.archive = self.root / "runtime.tar.gz"
        self.pack()

    def pack(self):
        with tarfile.open(self.archive, "w:gz") as tar:
            tar.add(self.runtime, arcname=".")

    def run_cli(self):
        return subprocess.run(
            [
                sys.executable,
                "-I",
                str(
                    getattr(
                        self, "entry", self.runtime / "bin/henukit-quizcraft-release.py"
                    )
                ),
                "capability",
                "--sha",
                SHA,
                "--release-dir",
                str(self.runtime),
                "--archive",
                str(self.archive),
            ],
            env={**os.environ, "HENUKIT_TRUST_ANCHOR": str(self.root)},
            capture_output=True,
            text=True,
        )

    def test_capability_is_exact_source_bound_and_closed(self):
        result = self.run_cli()
        self.assertEqual(result.returncode, 0, result.stderr)
        value = json.loads(result.stdout)
        self.assertTrue(value["training_compatible"])
        path = self.runtime / "release-gates/english-training-capabilities.json"
        path.write_text(json.dumps({**CAP, "release_sha": "b" * 40}))
        self.pack()
        result = self.run_cli()
        self.assertNotEqual(result.returncode, 0)

    def test_capability_tampering_and_partial_consumers_fail_closed(self):
        path = self.runtime / "release-gates/english-training-capabilities.json"
        for value in (
            {**CAP, "extra": True},
            {**CAP, "consumers": {"quizcraft": ["english-training-data-v1"]}},
        ):
            path.write_text(json.dumps(value))
            self.pack()
            self.assertNotEqual(self.run_cli().returncode, 0)
        path.write_text(json.dumps(CAP))
        self.pack()
        path.write_text(json.dumps({**CAP, "release_sha": "b" * 40}))
        self.assertNotEqual(self.run_cli().returncode, 0)


FixtureBase = type(
    "FixtureBase",
    (unittest.TestCase,),
    {
        k: v
        for source in (food.ReleaseCLI, practice.PracticeCLI)
        for k, v in source.__dict__.items()
        if not k.startswith(("test", "__"))
    },
)


class FullReleaseCLI(FixtureBase):
    # Inherit fixture construction only, not the original scoped deployment tests.
    def setUp(self):
        practice.PracticeCLI.setUp(self)
        self.runtime = self.runtime
        self.state = self.root / "full-state"
        self.state.mkdir()
        self.backups = self.root / "full-backups"
        self.backups.mkdir()
        (self.state / "prepared").mkdir()
        (self.state / "approvals").mkdir()
        (self.state / "rollback-contracts/pending").mkdir(parents=True)
        self.meta = self.backups / "platform.dump.meta"
        dump = self.backups / "platform.dump"
        dump.write_bytes(b"original backup")
        self.meta.write_text("release_sha=" + food.NEW + "\n")
        self.meta.chmod(0o600)
        (self.state / "prepared" / food.NEW).write_text(str(dump))
        (self.state / "approvals" / food.NEW).write_text(food.NEW)
        for path in (
            dump,
            self.state / "prepared" / food.NEW,
            self.state / "approvals" / food.NEW,
        ):
            path.chmod(0o600)
        contract = self.state / "rollback-contracts/pending" / food.NEW
        contract.write_text(
            "candidate_sha="
            + food.NEW
            + "\nprevious_sha="
            + food.OLD
            + "\n"
            + "test=value\n" * 6
        )
        contract.chmod(0o400)
        self.source_lock = self.root / "source.lock"
        self.source_lock.touch()
        (self.state / "watcher.lock").touch()
        previous = self.runtime.parent / food.OLD
        previous.mkdir(exist_ok=True)
        (previous / "RELEASE_SHA").write_text(food.OLD)
        for name in ("henukit-quizcraft-release.py", "henukit-quizcraft-database.py"):
            shutil.copyfile(SOURCE / name, self.runtime / "bin" / name)
        (self.runtime / "RELEASE_SHA").write_text(food.NEW)
        (self.runtime / "release-gates").mkdir(exist_ok=True)
        cap = {**CAP, "release_sha": food.NEW}
        (self.runtime / "release-gates/english-training-capabilities.json").write_text(
            json.dumps(cap)
        )
        core = SOURCE.parents[1] / "products/quizcraft/go-service"
        for path in (core / "db/migrations").glob("*.sql"):
            shutil.copyfile(path, self.runtime / "migrations/quizcraft" / path.name)
        (self.runtime / "migrations/quizcraft/000001.up.sql").unlink()
        for version in ("000011", "000012"):
            shutil.copyfile(
                core / f"db/release/backup-{version}.sql",
                self.runtime / f"release-gates/quizcraft-backup-{version}.sql",
            )
        state = json.loads((self.root / "fake-state.json").read_text())
        state.update(schema="000011", training=False)
        (self.root / "fake-state.json").write_text(json.dumps(state))
        adapter = (self.root / "bin/psql").read_text()
        adapter = adapter.replace(
            "elif name=='docker':",
            """elif name in ['quizcraft-release-state','quizcraft-migrate']:
 if name=='quizcraft-migrate':
  if os.environ.get('FAIL_MIGRATE'):sys.exit(1)
  state['schema']='000012';save();out({'applied':[{'version':'000012'}],'adopted':[],'skipped':[]})
 else:
  if os.environ.get('UNKNOWN_SCHEMA'):sys.exit(1)
  out({'format':'quizcraft-release-state-v1','database_identity':{'database':'quizcraft_v2','oid':42,'started':'2026-10-01','port':5432,'version':160015},'schema':{'version':state['schema'],'history':[],'catalog_sha256':'f'*64,'history_sha256':'e'*64},'requires_english_training_data_v1':state['training']})
elif name=='docker':""",
        )
        # The PostgreSQL adapter handles a streamed fixed verifier separately from
        # identity/snapshot queries, and the original OS boundary remains real.
        adapter = adapter.replace(
            "line=sys.stdin.readline()\n   print",
            """line=sys.stdin.readline()
   if 'COPY (' in line:
    print('@catalog\\t\\nquizcraft_schema_migrations\\t\\nquizcraft_schema_migrations\\t'+'a'*64)
    sys.exit(0)
   print""",
        )
        adapter = adapter.replace(
            "if '--command' not in args:",
            "if '--command' not in args and '-d' not in args and '--dbname' not in args:",
        )
        adapter = adapter.replace(
            "elif name=='runuser':",
            """elif name=='runuser':
  if '--dbname' in args and any('henukit_quizcraft_verify_' in a for a in args):
   if pathlib.Path(args[args.index('--')+1]).name=='psql':
    sys.stdin.read();print('@catalog\\t\\nquizcraft_schema_migrations\\t\\nquizcraft_schema_migrations\\t'+('b' if os.environ.get('BAD_CONTENT') else 'a')*64);sys.exit(0)
  if '--command' in args and 'other_backends' in args[-1]:out({'other_backends':1 if os.environ.get('EXTRA_WRITER') else 0});sys.exit(0)""",
        )
        adapter = adapter.replace(
            "elif args[0]=='inspect':",
            """elif args[0]=='stop':
  if not os.environ.get('FAIL_FREEZE'):
   c=next(c for c in state['containers'].values() if c['Id']==args[1]);c['State']['Running']=False;c['State']['Pid']=0;save()
 elif args[0]=='inspect':""",
        )
        for name in (
            "docker",
            "psql",
            "pg_dump",
            "pg_restore",
            "runuser",
            "createdb",
            "dropdb",
            "nsenter",
        ):
            (self.root / "bin" / name).write_text(adapter)
        for name in ("quizcraft-release-state", "quizcraft-migrate"):
            path = self.runtime / "bin" / name
            path.write_text(adapter)
            path.chmod(0o755)
        adapter = adapter.replace(
            "'started':'2026-10-01'",
            "'started':('2026-10-02' if state.get('database_changed') else '2026-10-01')",
        )
        adapter = adapter.replace(
            "if tool=='pg_restore' and os.environ.get('FAIL_RESTORE'):sys.exit(3)",
            "if tool=='pg_restore' and os.environ.get('FAIL_RESTORE'):sys.exit(3)\n  if tool=='pg_restore' and os.environ.get('DRIFT_DURING_BACKUP'):state['database_changed']=True;save()",
        )
        adapter = adapter.replace(
            "pathlib.Path(args[args.index('--file')+1]).write_bytes(b'PGDMP fixture archive')",
            "pathlib.Path(args[args.index('--file')+1]).write_bytes(b'PGDMP fixture archive')\n  if os.environ.get('RESTART_DURING_BACKUP'):\n   state['containers']['henukit-quizcraft-1']['State']['StartedAt']='2026-10-02T00:00:00Z';save()",
        )
        for name in (
            "docker",
            "psql",
            "pg_dump",
            "pg_restore",
            "runuser",
            "createdb",
            "dropdb",
            "nsenter",
        ):
            (self.root / "bin" / name).write_text(adapter)
        for name in ("quizcraft-release-state", "quizcraft-migrate"):
            (self.runtime / "bin" / name).write_text(adapter)
        self.repack()

    def full(self, action, flags=None, extra=None):
        args = [
            sys.executable,
            "-I",
            str(
                getattr(
                    self, "entry", self.runtime / "bin/henukit-quizcraft-release.py"
                )
            ),
            action,
            "--sha",
            food.NEW,
            "--previous-sha",
            food.OLD,
            "--release-dir",
            str(self.runtime),
            "--archive",
            str(self.artifacts / ("henukit-runtime-" + food.NEW + ".tar.gz")),
            "--state-root",
            str(self.state),
            "--env-file",
            str(self.envfile),
            "--backup-root",
            str(self.backups),
        ]
        args += extra or []
        env = {
            **os.environ,
            "PATH": str(self.root / "bin") + os.pathsep + os.environ["PATH"],
            "HENUKIT_TRUST_ANCHOR": str(self.root),
            "HENUKIT_SOURCE_DEPLOY_LOCK": str(self.source_lock),
            "TEST_ROOT": str(self.root),
            **(flags or {}),
        }
        # Existing fixtures use this same root variable for OS command adapters.
        env.update(self.env if hasattr(self, "env") else {})
        env.update(
            HENUKIT_TRUST_ANCHOR=str(self.root),
            HENUKIT_SOURCE_DEPLOY_LOCK=str(self.source_lock),
        )
        self.owner_environment = env
        wrapper = [
            "bash",
            "-c",
            'exec 6<"$1"; exec 7<"$2"; exec 9<"$3"; shift 3; exec "$@"',
            "hc651",
            str(self.state),
            str(self.source_lock),
            str(self.state / "watcher.lock"),
            *args,
        ]
        return subprocess.run(wrapper, env=env, capture_output=True, text=True)

    def test_normal_upgrade_and_precontent_rollback_preserve_environment(self):
        original = self.envfile.read_bytes()
        inode = self.envfile.stat().st_ino
        for action in ("begin", "migrate", "guard", "rollback"):
            result = self.full(action)
            self.assertEqual(result.returncode, 0, (action, result.stderr))
        self.assertEqual(self.envfile.read_bytes(), original)
        self.assertEqual(self.envfile.stat().st_ino, inode)

    def test_content_accepted_after_activation_blocks_old_consumers(self):
        result = self.full("begin")
        self.assertEqual(result.returncode, 0, result.stderr)
        state = json.loads((self.root / "fake-state.json").read_text())
        state["training"] = True
        (self.root / "fake-state.json").write_text(json.dumps(state))
        result = self.full("rollback")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(
            any(tool == "docker" and "up" in args for tool, args in self.calls())
        )

    def test_freeze_backup_restore_migration_and_unknown_state_fail_closed(self):
        for flag in (
            "FAIL_FREEZE",
            "FAIL_BACKUP",
            "FAIL_RESTORE",
            "BAD_CONTENT",
            "FAIL_MIGRATE",
            "EXTRA_WRITER",
            "UNKNOWN_SCHEMA",
        ):
            with self.subTest(flag=flag):
                case = FullReleaseCLI(
                    "test_normal_upgrade_and_precontent_rollback_preserve_environment"
                )
                case.setUp()
                try:
                    result = case.full("begin")
                    self.assertEqual(result.returncode, 0, result.stderr)
                    result = case.full("migrate", {flag: "1"})
                    self.assertNotEqual(result.returncode, 0, flag)
                    self.assertNotIn("password", result.stderr + result.stdout)
                    self.assertFalse(
                        (
                            case.state
                            / "quizcraft-releases"
                            / food.NEW
                            / "migrated.json"
                        ).exists()
                    )
                    self.assertFalse(
                        any(
                            tool == "docker" and "up" in args
                            for tool, args in case.calls()
                        )
                    )
                finally:
                    case.doCleanups()

    def test_missing_and_same_byte_replaced_evidence_never_resumes(self):
        for kind in ("receipt", "capability", "environment", "contract"):
            with self.subTest(kind=kind):
                case = FullReleaseCLI(
                    "test_normal_upgrade_and_precontent_rollback_preserve_environment"
                )
                case.setUp()
                try:
                    result = case.full("begin")
                    self.assertEqual(result.returncode, 0, result.stderr)
                    path = {
                        "receipt": case.state
                        / "quizcraft-releases"
                        / food.NEW
                        / "identity.json",
                        "capability": case.runtime
                        / "release-gates/english-training-capabilities.json",
                        "environment": case.envfile,
                        "contract": case.state
                        / "rollback-contracts/pending"
                        / food.NEW,
                    }[kind]
                    raw = path.read_bytes()
                    mode = path.stat().st_mode & 0o777
                    path.unlink()
                    path.write_bytes(raw)
                    path.chmod(mode)
                    result = case.full("migrate")
                    self.assertNotEqual(result.returncode, 0, kind)
                finally:
                    case.doCleanups()

    def test_interrupted_stop_reconciles_to_precontent_compatible_rollback(self):
        self.assertEqual(self.full("begin").returncode, 0)
        result = self.full("migrate", {"FAIL_BACKUP": "1"})
        self.assertNotEqual(result.returncode, 0)
        state = json.loads((self.root / "fake-state.json").read_text())
        self.assertFalse(state["containers"]["henukit-quizcraft-1"]["State"]["Running"])
        result = self.full("rollback")
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_training_facts_allow_only_the_original_compatible_previous_target(self):
        previous = self.runtime.parent / food.OLD
        (previous / "release-gates").mkdir(exist_ok=True)
        cap = previous / "release-gates/english-training-capabilities.json"
        cap.write_text(json.dumps({**CAP, "release_sha": food.OLD}))
        archive = self.root / "previous-runtime.tar.gz"
        with tarfile.open(archive, "w:gz") as tar:
            tar.add(previous, arcname=".")

        def file_identity(path):
            info = path.stat()
            value = {
                k: getattr(info, "st_" + k)
                for k in (
                    "dev",
                    "ino",
                    "uid",
                    "gid",
                    "mode",
                    "size",
                    "mtime_ns",
                    "ctime_ns",
                )
            }
            value["sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
            return value

        folder = self.state / "quizcraft-releases" / food.OLD
        folder.mkdir(parents=True)
        meta = self.backups / "previous.meta"
        meta.write_text("release_sha=" + food.OLD + "\n")
        meta.chmod(0o600)
        record = folder / "identity.json"
        record.write_text(
            json.dumps(
                {
                    "format": "henukit-quizcraft-release-v1",
                    "sha": food.OLD,
                    "metadata": str(meta),
                    "metadata_inode": [meta.stat().st_dev, meta.stat().st_ino],
                    "archive_path": str(archive),
                    "archive_identity": file_identity(archive),
                    "payload": {
                        "release-gates/english-training-capabilities.json": file_identity(
                            cap
                        )
                    },
                }
            )
        )
        record.chmod(0o400)
        accepted = folder / "accepted.json"
        accepted.write_text(
            json.dumps(
                {
                    "consumer_capability": file_identity(cap),
                    "state": {
                        "schema": {"version": "000012"},
                        "database_identity": {
                            "database": "quizcraft_v2",
                            "oid": 42,
                            "started": "2026-10-01",
                            "port": 5432,
                            "version": 160015,
                        },
                    },
                }
            )
        )
        accepted.chmod(0o400)
        with meta.open("a") as stream:
            for name, path in [("identity", record), ("accepted", accepted)]:
                stream.write(
                    "quizcraft_"
                    + name
                    + "="
                    + json.dumps(
                        file_identity(path), sort_keys=True, separators=(",", ":")
                    )
                    + "\n"
                )
        state = json.loads((self.root / "fake-state.json").read_text())
        state["training"] = True
        state["schema"] = "000012"
        state["database_changed"] = True
        (self.root / "fake-state.json").write_text(json.dumps(state))
        for action in ("begin", "migrate", "rollback"):
            result = self.full(action)
            self.assertEqual(result.returncode, 0, (action, result.stderr))
        accepted.unlink()
        result = self.full("rollback")
        self.assertNotEqual(result.returncode, 0)

    def test_installed_module_must_match_candidate_before_database_actions(self):
        installed = self.root / "installed"
        installed.mkdir()
        self.entry = installed / "henukit-quizcraft-release.py"
        shutil.copyfile(SOURCE / "henukit-quizcraft-release.py", self.entry)
        module = installed / "henukit-quizcraft-database.py"
        shutil.copyfile(SOURCE / "henukit-quizcraft-database.py", module)
        module.write_bytes(
            module.read_bytes() + b"\n# stale separately installed companion\n"
        )
        result = self.full("begin")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(
            any(
                tool in ("psql", "pg_dump", "runuser", "nsenter", "quizcraft-migrate")
                for tool, _ in self.calls()
            )
        )

    def test_identity_or_writer_drift_during_backup_stops_before_migration(self):
        for flag in ("DRIFT_DURING_BACKUP", "RESTART_DURING_BACKUP"):
            with self.subTest(flag=flag):
                case = FullReleaseCLI(
                    "test_normal_upgrade_and_precontent_rollback_preserve_environment"
                )
                case.setUp()
                try:
                    self.assertEqual(case.full("begin").returncode, 0)
                    result = case.full("migrate", {flag: "1"})
                    self.assertNotEqual(result.returncode, 0)
                    self.assertFalse(
                        any(tool == "quizcraft-migrate" for tool, _ in case.calls())
                    )
                finally:
                    case.doCleanups()

    def test_retained_dump_and_restore_proof_are_required_at_activation(self):
        for kind in ("dump", "proof"):
            with self.subTest(kind=kind):
                case = FullReleaseCLI(
                    "test_normal_upgrade_and_precontent_rollback_preserve_environment"
                )
                case.setUp()
                try:
                    for action in ("begin", "migrate"):
                        self.assertEqual(case.full(action).returncode, 0)
                    proof = json.loads(
                        (
                            case.state / "quizcraft-releases" / food.NEW / "backup.json"
                        ).read_text()
                    )
                    path = Path(
                        proof["path"] if kind == "dump" else proof["proof_path"]
                    )
                    raw = path.read_bytes()
                    path.unlink()
                    path.write_bytes(raw)
                    path.chmod(0o400)
                    self.assertNotEqual(case.full("guard").returncode, 0)
                finally:
                    case.doCleanups()

    def test_restarted_stopped_writer_cannot_reuse_prior_freeze(self):
        for action in ("begin", "migrate"):
            self.assertEqual(self.full(action).returncode, 0)
        state = json.loads((self.root / "fake-state.json").read_text())
        state["containers"]["henukit-quizcraft-1"]["State"]["StartedAt"] = (
            "2026-10-03T00:00:00Z"
        )
        (self.root / "fake-state.json").write_text(json.dumps(state))
        self.assertNotEqual(self.full("guard").returncode, 0)

    def test_degraded_helper_preserves_current_authority_and_uses_bound_rollback_input(
        self,
    ):
        rollback_env = self.root / "original-rollback.env"
        rollback_env.write_text("SECRET=previous-existing-value\n")
        rollback_env.chmod(0o600)
        contract = self.state / "rollback-contracts/pending" / food.NEW
        contract.chmod(0o600)
        contract.write_text(
            "candidate_sha="
            + food.NEW
            + "\nprevious_sha="
            + food.OLD
            + "\nmaterials_path_state=disabled\ndeploy_webhook_state=absent\nrollback_mode=degraded\nprevious_compose_sha256="
            + "c" * 64
            + "\nrollback_env_sha256="
            + hashlib.sha256(rollback_env.read_bytes()).hexdigest()
            + "\nmaterials_manifest_sha256="
            + "d" * 64
            + "\n"
        )
        contract.chmod(0o400)
        for action in ("begin", "migrate", "rollback"):
            result = self.full(action)
            self.assertEqual(result.returncode, 0, (action, result.stderr))
        previous = self.runtime.parent / food.OLD
        (previous / "bin").mkdir(exist_ok=True)
        (previous / "migrations/quizcraft").mkdir(parents=True, exist_ok=True)
        (previous / "materials-runtime").mkdir(exist_ok=True)
        reached = self.root / "degraded-installer-reached"
        installer = previous / "materials-runtime/install.sh"
        installer.write_text('#!/bin/sh\ntouch "$HC651_DEGRADED_REACHED"\nexit 47\n')
        installer.chmod(0o755)
        helper = previous / "bin/deploy-henukit-artifact.sh"
        shutil.copyfile(SOURCE / "deploy-henukit-artifact.sh", helper)
        helper.chmod(0o755)
        env = {
            **self.owner_environment,
            "HENUKIT_QUIZCRAFT_RELEASE_OWNER": str(
                self.runtime / "bin/henukit-quizcraft-release.py"
            ),
            "HENUKIT_QUIZCRAFT_CANDIDATE_SHA": food.NEW,
            "HENUKIT_QUIZCRAFT_PREVIOUS_SHA": food.OLD,
            "HENUKIT_QUIZCRAFT_RUNTIME": str(self.runtime),
            "HENUKIT_QUIZCRAFT_ARCHIVE": str(
                self.artifacts / ("henukit-runtime-" + food.NEW + ".tar.gz")
            ),
            "HENUKIT_QUIZCRAFT_STATE_ROOT": str(self.state),
            "HENUKIT_QUIZCRAFT_BACKUP_ROOT": str(self.backups),
            "HENUKIT_QUIZCRAFT_ENV_FILE": str(self.envfile),
            "HENUKIT_QUIZCRAFT_GUARD_ACTION": "guard-rollback",
            "HC651_DEGRADED_REACHED": str(reached),
        }
        wrapper = [
            "bash",
            "-c",
            'exec 6<"$1"; exec 7<"$2"; exec 9<"$3"; shift 3; exec "$@"',
            "hc651",
            str(self.state),
            str(self.source_lock),
            str(self.state / "watcher.lock"),
            str(helper),
            str(previous),
            str(rollback_env),
        ]
        docker = self.root / "bin/docker"
        source = docker.read_text().replace(
            "if name=='gh':",
            "if name=='docker' and args[0]=='compose' and 'up' in args and args[-1]=='postgres':sys.exit(0)\nif name=='docker' and args[0]=='compose' and 'exec' in args and 'postgres' in args:print('1');sys.exit(0)\nif name=='gh':",
        )
        docker.write_text(source)
        result = subprocess.run(wrapper, env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 47, result.stderr)
        self.assertTrue(reached.exists(), "real helper did not pass the rollback guard")
        self.assertEqual(self.envfile.read_text(), "SECRET=not-printed\n")
        reached.unlink()
        rollback_env.write_text("SECRET=changed\n")
        result = subprocess.run(wrapper, env=env, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(reached.exists())
