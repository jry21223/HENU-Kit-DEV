"""Exercise the exported-snapshot/isolated-restore seam with real PostgreSQL 16."""
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import sys
import tempfile
import time
from types import SimpleNamespace
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
BIN = next((path for path in (Path('/usr/lib/postgresql/16/bin'), Path('/opt/homebrew/opt/postgresql@16/bin')) if (path / 'initdb').exists()), None)


@unittest.skipUnless(BIN and os.geteuid() != 0, 'PostgreSQL 16 initdb and a non-root local user required')
class PostgreSQLSnapshot(unittest.TestCase):
    def test_isolated_restore_matches_exported_content_and_attempt_counts_with_concurrent_write(self):
        with tempfile.TemporaryDirectory(prefix='henukit-practice-pg16-') as directory:
            root = Path(directory).resolve()
            runtime = root / 'runtime'; runtime.mkdir()
            for name in ('deploy-henukit-food-scoped.py', 'deploy-henukit-practice-scoped.py'):
                shutil.copy(ROOT / name, runtime / name)
            with patch.object(sys, 'argv', ['test', '--test-root', str(root)]):
                spec = importlib.util.spec_from_file_location('practice_pg16_contract', runtime / 'deploy-henukit-practice-scoped.py')
                module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)
            env = {k: v for k, v in os.environ.items() if not k.startswith('PG')}
            def call(name, *args, **kwargs):
                return subprocess.run([str(BIN / name), *map(str, args)], env=env, capture_output=True, check=True, timeout=30, **kwargs)
            data = root / 'data'
            call('initdb', '-D', data, '--no-locale', '--encoding=UTF8', '--auth=trust', '--username=fixture_admin')
            with socket.socket() as listener:
                listener.bind(('127.0.0.1', 0)); port = listener.getsockname()[1]
            call('pg_ctl', '-D', data, '-l', root / 'server.log', '-o', f'-F -p {port} -h 127.0.0.1 -k {root}', '-w', 'start')
            try:
                env.update(PGHOST='127.0.0.1', PGPORT=str(port), PGUSER='fixture_admin')
                call('createdb', 'fixture_source')
                call('createdb', 'fixture_restore')
                tables = ('quizcraft_banks', 'quizcraft_bank_versions', 'quizcraft_questions', 'quizcraft_question_versions', 'quizcraft_bank_version_questions', 'quizcraft_practice_attempts', 'quizcraft_practice_sessions', 'quizcraft_workshop_audit_events')
                sql = ';'.join(f'CREATE TABLE {table}(id int); INSERT INTO {table} VALUES(1)' for table in tables)
                call('psql', '-X', '-v', 'ON_ERROR_STOP=1', '-d', 'fixture_source', '-c', sql)
                release = object.__new__(module.Release)
                release.env = env
                release.tools = {name: str(BIN / name) for name in ('psql', 'pg_dump', 'pg_restore')}
                release.database_url = f'postgres://fixture_admin@127.0.0.1:{port}/fixture_source?sslmode=disable'
                archive = root / 'snapshot.dump'
                with release.database_snapshot() as snapshot:
                    call('psql', '-X', '-d', 'fixture_source', '-c', 'INSERT INTO quizcraft_practice_attempts VALUES(2)')
                    call('pg_dump', '--format=custom', '--snapshot', snapshot['snapshot'], '--file', archive, '--dbname', release.database_url)
                self.assertIn(b'quizcraft_practice_attempts', call('pg_restore', '--list', archive).stdout)
                with archive.open('rb') as stream:
                    call('pg_restore', '--exit-on-error', '--no-owner', '--no-privileges', '--dbname', 'fixture_restore', stdin=stream)
                restored = json.loads(call('psql', '-XAt', '-d', 'fixture_restore', '-c', module.COUNTS).stdout)
                self.assertEqual(restored, snapshot['counts'])
                self.assertEqual(restored['attempts'], 1)
                self.assertEqual(call('psql', '-XAt', '-d', 'fixture_source', '-c', 'SELECT count(*) FROM quizcraft_practice_attempts').stdout.strip(), b'2')
            finally:
                call('pg_ctl', '-D', data, '-m', 'immediate', '-w', 'stop')


@unittest.skipUnless(shutil.which('docker'), 'Docker is required for the isolated PostgreSQL 17 Food contract')
class FoodPostgreSQL17(unittest.TestCase):
    def test_reused_food_engine_restores_real_container_database_and_records_separate_evidence(self):
        docker = shutil.which('docker')
        image = subprocess.run([docker, 'image', 'inspect', 'postgres:17-alpine'], capture_output=True, timeout=15)
        if image.returncode:
            self.skipTest('local postgres:17-alpine image unavailable; this test never pulls or changes shared containers')
        name = 'henukit-food-backup-contract-' + secrets.token_hex(6)
        subprocess.run([docker, 'run', '--detach', '--rm', '--name', name, '--network', 'none', '-e', 'POSTGRES_USER=fixture_admin', '-e', 'POSTGRES_PASSWORD=fixture-only-password', 'postgres:17-alpine'], capture_output=True, check=True, timeout=30)
        try:
            for _ in range(120):
                ready = subprocess.run([docker, 'exec', name, 'sh', '-ceu', '[ "$(cat /proc/1/comm)" = postgres ] && pg_isready -U fixture_admin'], capture_output=True, timeout=5)
                if ready.returncode == 0:
                    break
                time.sleep(0.25)
            else:
                self.fail('isolated PostgreSQL 17 did not become ready')
            def execute(*args):
                return subprocess.run([docker, 'exec', name, *args], capture_output=True, check=True, timeout=15).stdout
            self.assertTrue(execute('psql', '-U', 'fixture_admin', '-d', 'postgres', '-Atqc', 'SHOW server_version_num').strip().startswith(b'17'))
            execute('createdb', '-U', 'fixture_admin', 'food')
            execute('psql', '-U', 'fixture_admin', '-d', 'food', '-v', 'ON_ERROR_STOP=1', '-c', 'CREATE TABLE food_posts(id integer); INSERT INTO food_posts VALUES(1),(2),(3)')
            with tempfile.TemporaryDirectory(prefix='henukit-practice-food17-') as directory:
                root = Path(directory)
                spec = importlib.util.spec_from_file_location('food_pg17_contract', ROOT / 'deploy-henukit-food-scoped.py')
                module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)
                adapter = root / 'docker'
                adapter.write_text('#!' + sys.executable + '\nimport os,sys\nargs=sys.argv[1:]\nassert args[0]=="exec"\nargs[args.index("henukit-postgres-1")]=' + repr(name) + '\nos.execv(' + repr(docker) + ',[' + repr(docker) + ',*args])\n')
                adapter.chmod(0o700)
                release = object.__new__(module.Release)
                release.o = SimpleNamespace(sha='b' * 40)
                release.record = root / 'receipt.json'
                release.envfile = root / 'global.env'
                release.envfile.write_bytes(b'KEEP_CONFIGURATION=unchanged\n')
                release.envfile.chmod(0o600)
                release.tools = {'docker': str(adapter)}
                release.env = dict(os.environ)
                data = {'environment_sha256': module.digest(release.envfile)}
                module.Release.backup(release, data)
                archive = root / 'food.dump'
                self.assertEqual(data['backup_sha256'], module.digest(archive))
                self.assertEqual(data['backup_bytes'], archive.stat().st_size)
                self.assertEqual(data['restored_food_posts'], 3)
                self.assertEqual((root / 'configuration.backup').read_bytes(), release.envfile.read_bytes())
                remaining = execute('psql', '-U', 'fixture_admin', '-d', 'postgres', '-Atqc', "SELECT datname FROM pg_database WHERE datname LIKE 'henukit_food_verify_%'")
                self.assertEqual(remaining.strip(), b'')
        finally:
            subprocess.run([docker, 'rm', '--force', name], capture_output=True, check=True, timeout=15)


if __name__ == '__main__':
    unittest.main()
