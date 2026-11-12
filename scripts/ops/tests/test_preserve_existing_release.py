"""Run the preservation verifier with real files and a bounded Docker boundary."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

HELPER = Path(__file__).resolve().parents[1] / 'henukit-preserve-existing.py'
SHA = 'a' * 40
PERMISSIONS = ['account.tickets.read', 'account.tickets.reply', 'account.tickets.transition',
               'account.membership.write', 'account.points.adjust', 'account.orders.read',
               'account.orders.close', 'account.orders.refund']

class PreservationTest(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp(prefix='preserve-existing-')).resolve()
        self.addCleanup(shutil.rmtree, self.root)
        (self.root / 'bin').mkdir()
        self.source = self.root / ('c' * 40)
        self.source.mkdir()
        patches = self.source/'infra/epay-gateway/patches'
        patches.mkdir(parents=True)
        for path in (HELPER.parents[2]/'infra/epay-gateway/patches').glob('*.patch'):
            shutil.copyfile(path, patches/path.name)
        (self.source / 'RELEASE_SHA').write_text('c' * 40)
        (self.source / 'docker-compose.henukit.release.yml').write_text('fixture')
        self.config = {'services': {'account-portfolio': {'environment': {
            'ACCOUNT_PORTFOLIO_EASYPAY_ENABLED': '1', 'ACCOUNT_PORTFOLIO_EASYPAY_KEY': 'already-authorized-key',
            'ACCOUNT_PORTFOLIO_EASYPAY_PID': 'existing-pid', 'ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL': 'https://metaview.top/epay',
            'ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL': 'https://henukit.cn/api/v1/payment-providers/easypay/notifications',
            'ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL': 'https://henukit.cn/account/membership'}},
            'portal-gateway': {'environment': {'QUIZCRAFT_CORE_URL': 'http://quizcraft:10089'}}}}
        self.grants = {'role': {'id': 'existing-role', 'code': 'operations-operator', 'status': 'active', 'revision': 3},
                       'permissions': list(PERMISSIONS), 'bindings': [], 'audit': []}
        self.envfile = self.root / 'production.env'
        self.envfile.write_text('EXISTING_SECRET=already-authorized-key\n')
        self.envfile.chmod(0o600)
        self.receipt = self.root / 'receipt.json'
        self.save()
        docker = self.root / 'bin/docker'
        docker.write_text('''#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
r=Path(os.environ['FIXTURE_ROOT']); args=sys.argv[1:]
with (r/'calls').open('a') as log:log.write(' '.join(args)+'\\n')
config=json.loads((r/'config.json').read_text())
if args[0]=='compose':
 if 'config' in args: print(json.dumps(config))
 elif 'ps' in args: print(args[-1])
 else:sys.exit(71)
elif args[0]=='inspect':
 name=args[-1]; env=config['services'][name]['environment'].copy()
 if (r/'runtime-drift').exists():env[next(iter(env))]='wrong'
 print(json.dumps([{'Config':{'Env':[k+'='+v for k,v in env.items()]}}]))
elif args[0]=='exec':
 sql=sys.stdin.read()
 assert 'BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY' in sql
 assert not any(x in sql.upper() for x in ['INSERT ', 'UPDATE ', 'DELETE ', 'GRANT '])
 print(json.dumps(json.loads((r/'grants.json').read_text())))
else:sys.exit(72)
''')
        docker.chmod(0o755)
        ssh = self.root/'bin/ssh'
        ssh.write_text('''#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
r=Path(os.environ['FIXTURE_ROOT'])
with (r/'calls').open('a') as log:log.write('ssh readonly gateway\\n')
script=sys.stdin.read()
assert 'HENUKIT_EPAY_ENABLED' in script and 'urlopen' in script
assert not any(x in script for x in ['write_text', 'chmod', 'chown', 'systemctl', 'token_urlsafe'])
if (r/'gateway-drift').exists():sys.exit(1)
print(json.dumps({'env':{'ino':1,'sha256':'existing'},'patches':{'ino':2,'sha256':'existing'}}))
''')
        ssh.chmod(0o755)
        self.env = {**os.environ, 'PATH': str(self.root/'bin')+os.pathsep+os.environ['PATH'],
                    'FIXTURE_ROOT': str(self.root), 'HENUKIT_TRUST_ANCHOR': str(self.root)}

    def save(self):
        (self.root/'config.json').write_text(json.dumps(self.config))
        (self.root/'grants.json').write_text(json.dumps(self.grants))

    def run_helper(self, mode):
        return subprocess.run(['python3', '-I', str(HELPER), mode, '--sha', SHA,
                               '--env-file', str(self.envfile), '--receipt', str(self.receipt),
                               '--release-dir', str(self.source), '--role', 'operations-operator'],
                              env=self.env, capture_output=True, text=True)

    def test_capture_and_runtime_keep_authority_in_place_without_secrets_in_receipt(self):
        before = self.envfile.stat()
        result = self.run_helper('capture')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.run_helper('runtime').returncode, 0)
        self.assertEqual(self.envfile.stat().st_ino, before.st_ino)
        self.assertEqual(self.envfile.stat().st_mode, before.st_mode)
        self.assertEqual(self.envfile.read_text(), 'EXISTING_SECRET=already-authorized-key\n')
        self.assertNotIn('already-authorized-key', self.receipt.read_text())
        self.assertNotIn('grant-account', (self.root/'calls').read_text())

    def test_replaced_identical_file_and_missing_grants_fail_closed(self):
        result = self.run_helper('capture')
        self.assertEqual(result.returncode, 0, result.stderr)
        before = self.envfile.stat().st_ino
        replacement = self.root / 'replacement'
        replacement.write_bytes(self.envfile.read_bytes())
        replacement.chmod(0o600)
        replacement.replace(self.envfile)
        self.assertNotEqual(self.envfile.stat().st_ino, before)
        self.assertNotEqual(self.run_helper('check').returncode, 0)

    def test_missing_permission_cannot_create_a_preservation_receipt(self):
        self.grants['permissions'].remove('account.orders.refund')
        self.save()
        self.assertNotEqual(self.run_helper('capture').returncode, 0)
        self.assertFalse(self.receipt.exists())

    def test_runtime_and_candidate_configuration_drift_are_rejected(self):
        result = self.run_helper('capture')
        self.assertEqual(result.returncode, 0, result.stderr)
        (self.root/'runtime-drift').touch()
        self.assertNotEqual(self.run_helper('runtime').returncode, 0)
        (self.root/'runtime-drift').unlink()
        self.config['services']['account-portfolio']['environment']['ACCOUNT_PORTFOLIO_EASYPAY_KEY'] = 'a-different-existing-key'
        self.save()
        self.assertNotEqual(self.run_helper('candidate').returncode, 0)

    def test_permission_revision_or_binding_drift_is_rejected(self):
        result = self.run_helper('capture')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.grants['role']['revision'] = 4
        self.save()
        self.assertNotEqual(self.run_helper('check').returncode, 0)

    def test_wrong_mode_and_symlink_fail_without_a_receipt(self):
        self.envfile.chmod(0o644)
        self.assertNotEqual(self.run_helper('capture').returncode, 0)
        self.assertFalse(self.receipt.exists())
        self.envfile.chmod(0o600)
        destination = self.root/'original'
        self.envfile.rename(destination)
        self.envfile.symlink_to(destination)
        self.assertNotEqual(self.run_helper('capture').returncode, 0)
        self.assertFalse(self.receipt.exists())

    def test_formal_owner_prepares_and_activates_without_replacing_or_copying_credentials(self):
        self.owner_fixture()
        before = self.envfile.stat()
        result = self.run_owner()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.envfile.stat().st_ino, before.st_ino)
        self.assertEqual(self.envfile.stat().st_uid, before.st_uid)
        self.assertEqual(self.envfile.stat().st_gid, before.st_gid)
        self.assertEqual(self.envfile.stat().st_mode, before.st_mode)
        self.assertEqual(self.envfile.read_text(), 'EXISTING_SECRET=already-authorized-key\n')
        self.assertFalse(list(self.root.glob('.henukit-env*')))
        self.assertEqual((self.root/'state/last-activated-sha').read_text().strip(), SHA)
        calls = (self.root/'calls').read_text()
        self.assertNotIn('FORBIDDEN', calls)
        self.assertNotIn('grant-account', calls)
        self.assertNotIn('deploy-epay', calls)
        self.assertNotIn('openssl', calls)
        self.assertNotIn('bash -s', calls)
        self.assertNotIn('already-authorized-key', calls + result.stdout + result.stderr)

    def test_failed_preparation_does_not_replace_environment_or_create_approval(self):
        self.owner_fixture()
        self.env['FAIL_PREPARATION'] = '1'
        before = self.envfile.stat().st_ino
        result = self.run_owner()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.envfile.stat().st_ino, before)
        self.assertFalse((self.root/'state/approvals'/SHA).exists())
        self.assertFalse(list(self.root.glob('.henukit-env*')))

    def test_gateway_readiness_and_existing_content_changes_fail_closed(self):
        result = self.run_helper('capture')
        self.assertEqual(result.returncode, 0, result.stderr)
        (self.root/'gateway-drift').touch()
        self.assertNotEqual(self.run_helper('runtime').returncode, 0)
        (self.root/'gateway-drift').unlink()
        self.envfile.write_text('EXISTING_SECRET=a-different-already-present-key\n')
        self.assertNotEqual(self.run_helper('check').returncode, 0)

    def test_foreign_owner_is_refused(self):
        if os.geteuid() != 0:
            self.assertNotEqual(os.environ.get('HENUKIT_REQUIRE_FOREIGN_OWNER'), '1', 'required foreign-owner check needs root')
            self.skipTest('foreign-owner fixture requires root; Linux CI runs it with sudo')
        os.chown(self.envfile, 1, -1)
        self.assertNotEqual(self.run_helper('capture').returncode, 0)
        self.assertFalse(self.receipt.exists())

    def owner_fixture(self):
        state = self.root/'state'
        for name in ('approvals', 'prepared'):
            (state/name).mkdir(parents=True)
            (state/name).chmod(0o700)
        state.chmod(0o700)
        candidate = self.root/SHA
        shutil.copytree(self.source, candidate)
        (candidate/'RELEASE_SHA').write_text(SHA)
        inner = self.root/'compatible'
        outer = self.root/'current'
        inner.symlink_to(self.source)
        outer.symlink_to(inner)
        def metadata(path):
            info=path.lstat()
            return {k: getattr(info, 'st_'+k) for k in ('dev','ino','uid','gid')} | {'mode': info.st_mode & 0o7777}
        contract = {'format':'henukit-active-alias-forwarder-v1', 'release_root':str(self.root),
                    'inner':str(inner), 'outer':str(outer), 'outer_snapshot':metadata(outer)|{'text':str(inner)},
                    'parents':{str(self.root):metadata(self.root)}}
        (state/'active-alias-contract.json').write_text(json.dumps(contract))
        (state/'active-alias-contract.json').chmod(0o400)
        token = self.root/'token'
        token.write_text('fixture-token')
        token.chmod(0o600)
        for name in ('activate-henukit-release.sh', 'henukit-preserve-existing.py'):
            shutil.copyfile(HELPER.parent/name, self.root/'bin'/name)
            (self.root/'bin'/name).chmod(0o555)
        gh = self.root/'bin/gh'
        gh.write_text('#!/bin/sh\ncase "$*" in *issues*) echo closed;; *branches*) echo '+SHA+';; *) printf "123\\t1\\t'+SHA+'\\tcompleted\\tsuccess\\n";; esac\n')
        gh.chmod(0o755)
        watcher = self.root/'bin/watcher'
        watcher.write_text('''#!/usr/bin/env python3
import os,pathlib,sys
r=pathlib.Path(os.environ['FIXTURE_ROOT']);state=r/'state';sha='a'*40
assert os.environ['HENUKIT_PRESERVE_EXISTING_SHA']==sha
assert os.environ['HENUKIT_ENV_FILE']==os.environ['HENUKIT_ROLLBACK_ENV_FILE']
if os.environ.get('FAIL_PREPARATION'):sys.exit(1)
approval=state/'approvals'/sha
if approval.exists():
 approval.unlink();(r/'compatible').unlink();(r/'compatible').symlink_to(r/sha)
 (state/'last-activated-sha').write_text(sha)
else:
 dump=state/'baseline.dump';dump.write_text('restored backup');dump.chmod(0o600)
 meta=state/'baseline.dump.meta';meta.write_text('release_sha='+sha+'\\n');meta.chmod(0o600)
 prepared=state/'prepared'/sha;prepared.write_text(str(dump)+'\\n');prepared.chmod(0o600)
''')
        watcher.chmod(0o755)
        for program in ('cp', 'mv', 'chmod', 'awk', 'openssl'):
            original = shutil.which(program)
            wrapper = self.root/'bin'/program
            wrapper.write_text('#!/usr/bin/env python3\nimport os,sys\nfrom pathlib import Path\n'
                               'r=Path(os.environ["FIXTURE_ROOT"])\n'
                               'if "openssl" in sys.argv[0] or str(r/"production.env") in sys.argv[1:]:\n'
                               ' with (r/"calls").open("a") as log:log.write("FORBIDDEN mutation helper\\n")\n'
                               ' sys.exit(91)\n'
                               'os.execv('+repr(original)+',['+repr(original)+',*sys.argv[1:]])\n')
            wrapper.chmod(0o755)
        self.env.update(HENUKIT_STATE_ROOT=str(state), HENUKIT_RELEASE_ROOT=str(self.root),
                        HENUKIT_CURRENT_LINK=str(outer), HENUKIT_COMPAT_CURRENT_LINK=str(inner),
                        HENUKIT_ENV_FILE=str(self.envfile), GH_TOKEN_FILE=str(token),
                        HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE='operations-operator', HENUKIT_WATCHER=str(watcher))

    def run_owner(self):
        return subprocess.run([str(self.root/'bin/activate-henukit-release.sh'), SHA,
                               '--preserve-existing', '--execute'], env=self.env, text=True,
                              capture_output=True)

if __name__ == '__main__':
    unittest.main()
