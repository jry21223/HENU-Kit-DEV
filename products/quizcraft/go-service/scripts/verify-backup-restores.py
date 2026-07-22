#!/usr/bin/env python3
import hashlib
import json
import os
import pathlib
import secrets
import subprocess

import psycopg
from psycopg import sql
from psycopg.conninfo import conninfo_to_dict


evidence_path = pathlib.Path(os.environ['CUTOVER_GATE_EVIDENCE_FILE'])
admin_url = os.environ['CUTOVER_RESTORE_ADMIN_URL']
expected_run = os.environ['EXPECTED_MIGRATION_RUN_ID']
expected_head = int(os.environ['EXPECTED_SOURCE_HEAD'])
evidence = json.loads(evidence_path.read_text())
suffix = secrets.token_hex(6)
databases = {kind: f'quizcraft_restore_{kind}_{suffix}' for kind in ('legacy', 'go')}
for name in databases.values():
    if not name.startswith('quizcraft_restore_') or len(name) > 63:
        raise SystemExit('unsafe temporary restore database name')

connection_parameters = conninfo_to_dict(admin_url)
restore_environment = os.environ.copy()
mapping = {
    'host': 'PGHOST', 'port': 'PGPORT', 'user': 'PGUSER', 'password': 'PGPASSWORD',
    'sslmode': 'PGSSLMODE', 'sslrootcert': 'PGSSLROOTCERT', 'sslcert': 'PGSSLCERT', 'sslkey': 'PGSSLKEY',
}
for key, environment in mapping.items():
    if connection_parameters.get(key):
        restore_environment[environment] = connection_parameters[key]


def digest(path):
    value = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            value.update(chunk)
    return value.hexdigest()


with psycopg.connect(admin_url, autocommit=True) as admin:
    try:
        for kind, database in databases.items():
            backup = evidence['backups'][kind]
            path = pathlib.Path(backup['path'])
            if digest(path) != backup['sha256']:
                raise SystemExit(f'{kind} backup hash changed before restore')
            admin.execute(sql.SQL('CREATE DATABASE {}').format(sql.Identifier(database)))
            subprocess.run(
                ['pg_restore', '--exit-on-error', '--no-owner', '--no-privileges', '--dbname', database, str(path)],
                check=True, env=restore_environment, stdout=subprocess.DEVNULL,
            )

        restore_parameters = connection_parameters.copy()
        for kind, database in databases.items():
            restore_parameters['dbname'] = database
            with psycopg.connect(**restore_parameters) as restored:
                restored.execute('SET TRANSACTION READ ONLY')
                if kind == 'legacy':
                    tables = ('question_banks', 'bank_questions', 'feedbacks', 'quizcraft_migration_events')
                else:
                    tables = ('quizcraft_banks', 'quizcraft_questions', 'quizcraft_feedbacks', 'quizcraft_migration_runs')
                for table in tables:
                    if restored.execute('SELECT to_regclass(%s)', (f'public.{table}',)).fetchone()[0] is None:
                        raise SystemExit(f'{kind} restore is missing {table}')
                for table in tables[:2]:
                    count = restored.execute(sql.SQL('SELECT count(*) FROM {}').format(sql.Identifier(table))).fetchone()[0]
                    if count <= 0:
                        raise SystemExit(f'{kind} restore has no rows in {table}')
                if kind == 'legacy':
                    head = restored.execute('SELECT COALESCE(max(event_id), 0) FROM quizcraft_migration_events').fetchone()[0]
                    if head != expected_head:
                        raise SystemExit('legacy restored cursor does not match the frozen source head')
                else:
                    row = restored.execute(
                        'SELECT caught_up_event_id, state FROM quizcraft_migration_runs WHERE id = %s', (expected_run,),
                    ).fetchone()
                    if row != (expected_head, 'passed'):
                        raise SystemExit('Go restored migration run does not match the accepted cutover')
    finally:
        for database in databases.values():
            admin.execute('SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = %s', (database,))
            admin.execute(sql.SQL('DROP DATABASE IF EXISTS {}').format(sql.Identifier(database)))
