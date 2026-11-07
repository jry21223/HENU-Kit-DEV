"""Notice's single ordinary URL through the original publisher's CLI seam.

Only Docker is a fake system boundary. The environment, durable receipt,
inherited locks, atomic replacement and recovery are real isolated files.
"""
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

SOURCE = Path(__file__).resolve().parents[1] / "henukit-notice-url-transaction.py"
SHA = "a" * 40
URL = "http://notice:8094"
ORIGINAL = (b"# retained comments\r\nNOTICE_API_URL=''\r\n"
            b"NOTICE_SUMMARY_URL=\r\nFOOD_SUMMARY_URL=\r\n"
            b"LIBRARY_SUMMARY_URL=\r\nQUIZCRAFT_SUMMARY_URL=\r\n"
            b"UNCHANGED_VALUE='opaque fixture only'\r\n")
META_KEYS = ("dev", "ino", "uid", "gid", "mode", "size", "mtime_ns")

def metadata(path):
    s = path.stat()
    return dict(zip(META_KEYS, (s.st_dev, s.st_ino, s.st_uid, s.st_gid,
                              stat.S_IMODE(s.st_mode), s.st_size, s.st_mtime_ns)))

FAKE_DOCKER = r'''#!/usr/bin/env python3
import json,os,pathlib,sys
root=pathlib.Path(os.environ['NOTICE_FIXTURE']);args=sys.argv[1:]
with (root/'calls.jsonl').open('a') as f:f.write(json.dumps(args)+'\n')
p=root/'docker-state.json';s=json.loads(p.read_text())
def tag_id(tag):return s.get('tag_ids',{}).get(tag,s.get('tag_image_id','sha256:fixture'))
if args[:2]==['image','inspect']:
    assert args[2:4]==['--format','{{.Id}}'];print(tag_id(args[-1]));sys.exit(0)
if args[0]=='compose':
    assert os.environ.get('RELEASE_SHA')=='a'*40
    assert os.environ.get('PORTAL_VERSION')=='a'*40
    assert os.environ.get('PORTAL_DEPLOYED_AT')
    assert 'NOTICE_API_URL' not in os.environ
    if 'config' in args:
        s['config_count']=s.get('config_count',0)+1
        if s.get('replace_source_on_config')==s['config_count']:
            source=root/'source.lock';before=source.stat();replacement=root/'new-source.lock'
            replacement.write_bytes(b'');replacement.chmod(0o640);os.replace(replacement,source)
            after=source.stat();s['source_replacement']={'old_ino':before.st_ino,'new_ino':after.st_ino,
                'same_profile':(before.st_uid,before.st_gid,before.st_mode)==(after.st_uid,after.st_gid,after.st_mode)}
        if s.get('retag_on_config')==s['config_count']:s['tag_image_id']='sha256:retagged-fixture'
        p.write_text(json.dumps(s));sys.exit(0)
    if 'ps' in args:print(s['id']);sys.exit(0)
    assert args[-8:]==['up','-d','--no-deps','--force-recreate','--no-build','--pull','never','console-gateway'], args
    s['id']='gateway-'+str(s['count']+1);s['count']+=1
    s['image_id']=tag_id('henukit-console-gateway:'+s['sha'])
    s['health']='healthy' if s.get('failure')!='unhealthy' else 'unhealthy'
    p.write_text(json.dumps(s))
    sys.exit(1 if s.get('failure')=='after-recreate' else 0)
if args[0]=='inspect':
    assert '.Config.Env' not in ' '.join(args)
    print('|'.join([s['id'], 'henukit-console-gateway:'+s['sha'], s.get('image_id','sha256:fixture'),
                    'running',s['health'],'console-gateway']))
    sys.exit(0)
raise SystemExit('unexpected command')
'''

class NoticeURLTests(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp(prefix="henukit-notice-url-"))
        self.root.chmod(0o700)
        self.state = self.root / "state"; self.state.mkdir(mode=0o700)
        self.releases = self.root / "releases"; self.releases.mkdir(mode=0o700)
        self.runtime = self.releases / SHA; self.runtime.mkdir(mode=0o700)
        (self.runtime / "RELEASE_SHA").write_text(SHA + "\n")
        (self.runtime / "docker-compose.henukit.release.yml").write_text(
            "services:\n  console-gateway:\n    image: henukit-console-gateway:"+SHA+"\n"
            "    environment:\n      NOTICE_API_URL: ${NOTICE_API_URL-http://notice:8094}\n")
        (self.state / "last-activated-sha").write_text(SHA + "\n")
        (self.state / "last-activated-sha").chmod(0o600)
        self.inner = self.root / "current"; self.inner.symlink_to(self.runtime)
        self.outer = self.root / "outer"; self.outer.symlink_to(self.inner)
        self.env = self.root / "production.env"; self.env.write_bytes(ORIGINAL); self.env.chmod(0o600)
        self.source_lock = self.root / "source.lock"; self.source_lock.touch(mode=0o640)
        self.source_lock.chmod(0o640)  # exact original profile, independent of caller umask
        for name in ("watcher.lock", "direct-deploy.lock"):
            (self.state / name).touch(mode=0o600)
        self.plan = self.root / "plan.json"
        self.write_plan()
        self.bin = self.root / "bin"; self.bin.mkdir(mode=0o700)
        (self.bin / "docker").write_text(FAKE_DOCKER); (self.bin / "docker").chmod(0o700)
        (self.root / "docker-state.json").write_text(json.dumps(
            {"id":"gateway-original", "sha":SHA, "health":"healthy", "count":0}))
        (self.root / "calls.jsonl").touch()

    def tearDown(self):
        shutil.rmtree(self.root)

    def write_plan(self, **updates):
        value = {"format":"henukit-notice-url-plan-v1", "release_sha":SHA,
                 "environment_path":str(self.env), "environment_metadata":metadata(self.env),
                 "environment_sha256":hashlib.sha256(self.env.read_bytes()).hexdigest(),
                 "expected_value":"", "value":URL}
        value.update(updates); self.plan.write_text(json.dumps(value)); self.plan.chmod(0o600)

    def call(self, mode, *, locked=True, evidence=None, baseline=None, fault=None):
        args = [str(SOURCE), "--mode", mode, "--sha", SHA,
                "--env-file", str(self.env), "--state-root", str(self.state),
                "--release-root", str(self.releases), "--inner", str(self.inner),
                "--outer", str(self.outer), "--source-lock", str(self.source_lock),
                "--plan", str(self.plan)]
        if evidence: args += ["--evidence", str(evidence)]
        if baseline: args += ["--baseline-env", str(baseline)]
        # Exec wrapper is only the original owner's inherited lock protocol.
        wrapper = """import fcntl,os,sys
paths=sys.argv[1:5]
fds=[]
for p in paths:
 f=os.open(p,os.O_RDONLY);fds.append(fcntl.fcntl(f,fcntl.F_DUPFD,20));os.close(f)
for fd,src in zip((8,9,6,7),fds):
 os.dup2(src,fd,inheritable=True);fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
for fd in fds:os.close(fd)
if os.environ.get('NOTICE_FS_FAULT'):
 import runpy,pathlib,stat,subprocess
 args=sys.argv[5:];envpath=pathlib.Path(args[args.index('--env-file')+1]);original=os.replace
 def replace(source,target):
  if pathlib.Path(target)==envpath:
   if os.environ['NOTICE_FS_FAULT']=='after-env':original(source,target)
   raise OSError('fixture interrupted replacement')
  return original(source,target)
 if os.environ['NOTICE_FS_FAULT']=='block-env-fsync':
  original_fsync=os.fsync;parent=envpath.parent.stat()
  def fsync(fd):
   info=os.fstat(fd)
   if stat.S_ISDIR(info.st_mode) and (info.st_dev,info.st_ino)==(parent.st_dev,parent.st_ino):
    raise OSError('fixture interrupted environment directory fsync')
   return original_fsync(fd)
  os.fsync=fsync
 elif os.environ['NOTICE_FS_FAULT']=='timeout-up':
  original_run=subprocess.run
  def run(command,*positional,**keywords):
   if command[0]=='docker' and 'up' in command:
    assert keywords.get('timeout')==60
    raise subprocess.TimeoutExpired(command,60,output='fixture-sensitive-output-must-not-leak')
   return original_run(command,*positional,**keywords)
  subprocess.run=run
 else:os.replace=replace
 sys.argv=args;runpy.run_path(args[0],run_name='__main__')
else:os.execv(sys.executable,[sys.executable,'-I',*sys.argv[5:]])
"""
        command = [sys.executable, "-I", *args]
        if locked:
            command = [sys.executable, "-I", "-c", wrapper,
                       str(self.state/"direct-deploy.lock"), str(self.state/"watcher.lock"),
                       str(self.state), str(self.source_lock), *args]
        return subprocess.run(command, text=True, capture_output=True,
                              env={**os.environ, "PATH":str(self.bin)+os.pathsep+os.environ["PATH"],
                                   "NOTICE_FIXTURE":str(self.root),"NOTICE_FS_FAULT":fault or ""})

    def calls(self):
        return [json.loads(line) for line in (self.root/"calls.jsonl").read_text().splitlines()]

    def receipt(self):
        return json.loads((self.state/"notice-url"/SHA/"receipt.json").read_text())

    def test_only_url_changes_and_health_is_not_business_acceptance(self):
        before = metadata(self.env)
        result = self.call("apply")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.env.read_bytes(), ORIGINAL.replace(b"NOTICE_API_URL=''", b"NOTICE_API_URL="+URL.encode()))
        self.assertEqual({k:metadata(self.env)[k] for k in ("uid","gid","mode")},
                         {k:before[k] for k in ("uid","gid","mode")})
        self.assertEqual(self.receipt()["phase"], "awaiting-business-acceptance")
        self.assertFalse(self.receipt().get("business_accepted", False))
        ups = [a for a in self.calls() if "up" in a]
        self.assertEqual(len(ups), 1)
        self.assertEqual(ups[0][-1], "console-gateway")
        self.assertIn("--no-build", ups[0])

    def test_business_failure_restores_only_url_after_temporary_backup_is_gone(self):
        applied = self.call("apply")
        self.assertEqual(applied.returncode, 0, applied.stderr)
        # The original activator's full-env temporary copy is deliberately absent.
        self.assertEqual(list(self.root.glob(".henukit-env.backup.*")), [])
        result = self.call("restore")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.env.read_bytes(), ORIGINAL)
        self.assertEqual(self.receipt()["phase"], "restored")
        self.assertEqual(len([a for a in self.calls() if "up" in a]), 2)
        repeated = self.call("restore")
        self.assertEqual(repeated.returncode, 0, repeated.stderr)
        self.assertEqual(len([a for a in self.calls() if "up" in a]), 2)

    def test_unknown_command_is_reconciled_before_any_retry(self):
        path = self.root/"docker-state.json"
        state = json.loads(path.read_text()); state["failure"] = "after-recreate"; path.write_text(json.dumps(state))
        first = self.call("apply")
        self.assertNotEqual(first.returncode, 0)
        self.assertEqual(self.receipt()["phase"], "applying")
        retry = self.call("apply")
        self.assertNotEqual(retry.returncode, 0)
        self.assertEqual(len([a for a in self.calls() if "up" in a]), 1)
        reconciled = self.call("reconcile")
        self.assertEqual(reconciled.returncode, 0, reconciled.stderr)
        self.assertEqual(self.receipt()["phase"], "awaiting-business-acceptance")
        repeated = self.call("apply")
        self.assertEqual(repeated.returncode, 0, repeated.stderr)
        self.assertEqual(len([a for a in self.calls() if "up" in a]), 1)

    def test_unhealthy_recreated_gateway_can_be_explicitly_restored(self):
        path = self.root/"docker-state.json"
        state = json.loads(path.read_text()); state["failure"] = "unhealthy"; path.write_text(json.dumps(state))
        result = self.call("apply")
        self.assertNotEqual(result.returncode, 0)
        reconciled = self.call("reconcile")
        self.assertEqual(reconciled.returncode, 0, reconciled.stderr)
        self.assertEqual(self.receipt()["phase"], "applied-unhealthy")
        state = json.loads(path.read_text()); state.pop("failure"); path.write_text(json.dumps(state))
        restored = self.call("restore")
        self.assertEqual(restored.returncode, 0, restored.stderr)
        self.assertEqual(self.env.read_bytes(), ORIGINAL)

    def test_drift_and_missing_four_locks_cannot_mutate_environment(self):
        denied = self.call("apply",locked=False)
        self.assertNotEqual(denied.returncode, 0)
        self.assertEqual(self.env.read_bytes(), ORIGINAL)
        self.assertEqual(self.calls(), [])
        self.env.write_bytes(ORIGINAL+b"FOREIGN_CHANGE=keep\r\n")
        denied = self.call("apply")
        self.assertNotEqual(denied.returncode, 0)
        self.assertEqual(self.env.read_bytes(), ORIGINAL+b"FOREIGN_CHANGE=keep\r\n")
        self.assertEqual(self.calls(), [])

    def test_restore_refuses_foreign_environment_change(self):
        self.assertEqual(self.call("apply").returncode, 0)
        candidate = self.env.read_bytes()+b"FOREIGN_CHANGE=keep\r\n"
        self.env.write_bytes(candidate)
        count = len([a for a in self.calls() if "up" in a])
        result = self.call("restore")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.env.read_bytes(), candidate)
        self.assertEqual(len([a for a in self.calls() if "up" in a]), count)

    def test_manual_business_acceptance_is_a_separate_bound_receipt(self):
        self.assertEqual(self.call("apply").returncode, 0)
        evidence = self.root/"business-get.json"
        evidence.write_text(json.dumps({"format":"henukit-notice-business-acceptance-v1", "release_sha":SHA,
            "plan_sha256":self.receipt()["plan_sha256"], "route":"/api/v1/notices",
            "method":"GET", "session":"normal-authorized-browser", "result":"valid-owner-snapshot",
            "checked_at":"2026-11-08T00:00:00Z"})); evidence.chmod(0o600)
        result = self.call("accept",evidence=evidence)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.receipt()["phase"], "accepted")
        self.assertEqual(self.receipt()["business_evidence_sha256"], hashlib.sha256(evidence.read_bytes()).hexdigest())
        self.assertEqual(len([a for a in self.calls() if "up" in a]), 1)

    def test_new_sha_stages_after_baseline_backup_before_full_activation(self):
        old_sha = "b" * 40
        old = self.releases/old_sha; old.mkdir(mode=0o700)
        (old/"RELEASE_SHA").write_text(old_sha+"\n")
        (old/"docker-compose.henukit.release.yml").write_text("fixture old compose\n")
        self.inner.unlink(); self.inner.symlink_to(old)
        (self.state/"last-activated-sha").write_text(old_sha+"\n")
        baseline = self.root/".henukit-env.backup.12345"
        baseline.write_bytes(ORIGINAL); baseline.chmod(0o600)
        # The original activator may already have made its unrelated, existing
        # Account URL updates. The adapter must preserve them byte-for-byte.
        self.env.write_bytes(ORIGINAL+b"EXISTING_ACTIVATOR_CHANGE=fixture\r\n")
        path = self.root/"docker-state.json"
        state = json.loads(path.read_text()); state.update({"sha":old_sha,"image_id":"sha256:old-release",
            "tag_ids":{"henukit-console-gateway:"+old_sha:"sha256:old-release",
                       "henukit-console-gateway:"+SHA:"sha256:new-release"}}); path.write_text(json.dumps(state))
        staged = self.call("stage",baseline=baseline)
        self.assertEqual(staged.returncode, 0, staged.stderr)
        self.assertEqual(self.receipt()["phase"], "env-staged")
        self.assertEqual(len([a for a in self.calls() if "up" in a]), 0)
        self.assertTrue(self.env.read_bytes().endswith(b"EXISTING_ACTIVATOR_CHANGE=fixture\r\n"))
        self.inner.unlink(); self.inner.symlink_to(self.runtime)
        (self.state/"last-activated-sha").write_text(SHA+"\n")
        state.update({"sha":SHA,"id":"gateway-full-release","image_id":"sha256:new-release"}); path.write_text(json.dumps(state))
        baseline.unlink()  # normal activation cleanup must not lose URL preimage
        finished = self.call("health")
        self.assertEqual(finished.returncode, 0, finished.stderr)
        self.assertEqual(self.receipt()["phase"], "awaiting-business-acceptance")
        self.assertNotEqual(self.receipt()['before_gateway']['image_id'],self.receipt()['after_gateway']['image_id'])
        restored = self.call("restore")
        self.assertEqual(restored.returncode, 0, restored.stderr)
        self.assertEqual(self.env.read_bytes(), ORIGINAL+b"EXISTING_ACTIVATOR_CHANGE=fixture\r\n")

    def test_failed_full_activation_is_observed_without_replaying_or_rewriting_its_rollback(self):
        old_sha = "b"*40
        old = self.releases/old_sha; old.mkdir(mode=0o700)
        (old/"RELEASE_SHA").write_text(old_sha+"\n")
        (old/"docker-compose.henukit.release.yml").write_text("fixture old compose\n")
        self.inner.unlink(); self.inner.symlink_to(old)
        (self.state/"last-activated-sha").write_text(old_sha+"\n")
        baseline = self.root/".henukit-env.backup.12345"; baseline.write_bytes(ORIGINAL); baseline.chmod(0o600)
        state_path = self.root/"docker-state.json"
        state = json.loads(state_path.read_text()); state["sha"] = old_sha; state_path.write_text(json.dumps(state))
        self.assertEqual(self.call("stage",baseline=baseline).returncode, 0)
        os.replace(baseline,self.env) # original activator's existing full-env cleanup
        before = metadata(self.env)
        interrupted = self.call("reconcile",fault="block-env-fsync")
        self.assertNotEqual(interrupted.returncode,0)
        self.assertEqual(self.receipt()["phase"],"env-staged")
        result = self.call("reconcile")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.receipt()["phase"], "full-release-restored")
        self.assertEqual(metadata(self.env), before)
        self.assertEqual(len([a for a in self.calls() if "up" in a]), 0)

    def test_pending_full_release_blocks_url_recovery_without_touching_its_environment(self):
        self.assertEqual(self.call("apply").returncode, 0)
        pending = self.state/"rollback-contracts"/"pending"; pending.mkdir(parents=True,mode=0o700)
        (pending/("b"*40)).write_text("fixture pending full-release authority\n")
        candidate = self.env.read_bytes()
        result = self.call("restore")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.env.read_bytes(), candidate)
        self.assertEqual(len([a for a in self.calls() if "up" in a]), 1)

    def test_atomic_replace_interruption_is_reconciled_without_replaying_a_gateway_command(self):
        for fault,expected in (("before-env","not-applied"),("after-env","environment-applied")):
            with self.subTest(fault=fault):
                if (self.state/"notice-url").exists():
                    shutil.rmtree(self.state/"notice-url")
                self.env.write_bytes(ORIGINAL); self.write_plan()
                interrupted = self.call("apply",fault=fault)
                self.assertNotEqual(interrupted.returncode, 0)
                self.assertEqual(self.receipt()["phase"], "prepared")
                observed = self.call("reconcile")
                self.assertEqual(observed.returncode, 0, observed.stderr)
                self.assertEqual(self.receipt()["phase"], expected)
                self.assertEqual(len([a for a in self.calls() if "up" in a]), 0)
                if fault == "after-env":
                    restored = self.call("restore")
                    self.assertEqual(restored.returncode, 0, restored.stderr)
                    self.assertEqual(self.env.read_bytes(), ORIGINAL)

    def test_unapplied_attempt_can_retry_only_after_its_read_only_reconciliation(self):
        self.assertNotEqual(self.call("apply",fault="before-env").returncode, 0)
        self.assertNotEqual(self.call("apply").returncode, 0)
        self.assertEqual(self.call("reconcile").returncode, 0)
        self.assertEqual(self.receipt()["phase"], "not-applied")
        retried = self.call("apply")
        self.assertEqual(retried.returncode, 0, retried.stderr)
        self.assertEqual(self.receipt()["phase"], "awaiting-business-acceptance")
        self.assertEqual(len([a for a in self.calls() if "up" in a]), 1)
        self.assertEqual(len(list((self.state/"notice-url"/SHA).glob("reconciled-*.json"))), 1)

    def test_restore_replace_interruption_resumes_without_rewriting_an_already_restored_url(self):
        for fault,phase in (("before-env","awaiting-business-acceptance"),("after-env","restore-environment-applied")):
            with self.subTest(fault=fault):
                if (self.state/"notice-url").exists():shutil.rmtree(self.state/"notice-url")
                self.env.write_bytes(ORIGINAL); self.write_plan()
                self.assertEqual(self.call("apply").returncode, 0)
                interrupted = self.call("restore",fault=fault)
                self.assertNotEqual(interrupted.returncode, 0)
                self.assertEqual(self.receipt()["phase"], "restore-prepared")
                observed = self.call("reconcile")
                self.assertEqual(observed.returncode, 0, observed.stderr)
                self.assertEqual(self.receipt()["phase"], phase)
                restored_metadata = metadata(self.env)
                finished = self.call("restore")
                self.assertEqual(finished.returncode, 0, finished.stderr)
                self.assertEqual(self.env.read_bytes(), ORIGINAL)
                if fault == "after-env":self.assertEqual(metadata(self.env),restored_metadata)

    def test_full_release_preflight_is_read_only_and_requires_the_exact_original_inode(self):
        result = self.call("preflight",locked=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls(), [])
        other = self.root/"replacement.env"; other.write_bytes(ORIGINAL); other.chmod(0o600)
        os.replace(other,self.env)
        result = self.call("preflight",locked=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.env.read_bytes(), ORIGINAL)
        self.assertEqual(self.calls(), [])

    def test_original_source_lock_rejects_private_mode_and_nonempty_replacement(self):
        for mode,content in ((0o600,b""),(0o640,b"foreign lock content")):
            with self.subTest(mode=oct(mode),content_length=len(content)):
                self.source_lock.chmod(mode); self.source_lock.write_bytes(content)
                result = self.call("apply")
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.env.read_bytes(), ORIGINAL)
                self.assertEqual(self.calls(), [])

    def test_reconciliation_must_sync_owned_environment_rename_before_publishing_next_phase(self):
        for action,prepared in (("apply","prepared"),("restore","restore-prepared")):
            with self.subTest(action=action):
                if (self.state/"notice-url").exists():shutil.rmtree(self.state/"notice-url")
                self.env.write_bytes(ORIGINAL); self.write_plan()
                if action == "restore":self.assertEqual(self.call("apply").returncode,0)
                self.assertNotEqual(self.call(action,fault="after-env").returncode,0)
                self.assertEqual(self.receipt()["phase"],prepared)
                observed = self.call("reconcile",fault="block-env-fsync")
                self.assertNotEqual(observed.returncode,0)
                self.assertEqual(self.receipt()["phase"],prepared)
                recovered = self.call("reconcile")
                self.assertEqual(recovered.returncode,0,recovered.stderr)

    def test_gateway_timeout_keeps_unknown_receipt_and_does_not_leak_or_replay(self):
        result = self.call("apply",fault="timeout-up")
        self.assertNotEqual(result.returncode,0)
        self.assertIn("timed out",result.stderr)
        self.assertNotIn("fixture-sensitive-output",result.stderr)
        self.assertEqual(self.receipt()["phase"],"applying")
        self.assertNotEqual(self.call("apply").returncode,0)
        self.assertEqual(len([a for a in self.calls() if "up" in a]),0)

    def test_source_lock_replaced_during_config_blocks_all_environment_and_gateway_writes(self):
        path=self.root/'docker-state.json';state=json.loads(path.read_text());state['replace_source_on_config']=1;path.write_text(json.dumps(state))
        result=self.call('apply')
        self.assertNotEqual(result.returncode,0)
        self.assertEqual(self.env.read_bytes(),ORIGINAL)
        self.assertEqual(len([a for a in self.calls() if 'up' in a]),0)
        proof=json.loads(path.read_text())['source_replacement']
        self.assertNotEqual(proof['old_ino'],proof['new_ino'])
        self.assertTrue(proof['same_profile'])

    def test_source_lock_replaced_after_environment_write_keeps_unknown_without_up(self):
        path=self.root/'docker-state.json';state=json.loads(path.read_text());state['replace_source_on_config']=2;path.write_text(json.dumps(state))
        result=self.call('apply')
        self.assertNotEqual(result.returncode,0)
        self.assertEqual(self.receipt()['phase'],'applying')
        self.assertEqual(len([a for a in self.calls() if 'up' in a]),0)
        self.assertNotEqual(self.call('apply').returncode,0)
        self.assertEqual(len([a for a in self.calls() if 'up' in a]),0)

    def test_restore_lease_drift_after_environment_restore_cannot_recreate_or_publish_completion(self):
        self.assertEqual(self.call('apply').returncode,0)
        path=self.root/'docker-state.json';state=json.loads(path.read_text());state.update({'config_count':0,'replace_source_on_config':1});path.write_text(json.dumps(state))
        result=self.call('restore')
        self.assertNotEqual(result.returncode,0)
        self.assertEqual(self.receipt()['phase'],'restoring')
        self.assertEqual(self.env.read_bytes(),ORIGINAL)
        self.assertEqual(len([a for a in self.calls() if 'up' in a]),1)
        self.assertNotEqual(self.call('apply').returncode,0)
        self.assertEqual(len([a for a in self.calls() if 'up' in a]),1)

    def test_retagged_same_sha_is_rejected_before_any_environment_write(self):
        path=self.root/'docker-state.json';state=json.loads(path.read_text());state['tag_image_id']='sha256:retagged-fixture';path.write_text(json.dumps(state))
        before=metadata(self.env);result=self.call('apply')
        self.assertNotEqual(result.returncode,0)
        self.assertEqual(self.env.read_bytes(),ORIGINAL)
        self.assertEqual(metadata(self.env),before)
        self.assertEqual(len([a for a in self.calls() if 'up' in a]),0)

    def test_retag_during_config_is_rejected_before_environment_write(self):
        path=self.root/'docker-state.json';state=json.loads(path.read_text());state['retag_on_config']=1;path.write_text(json.dumps(state))
        before=metadata(self.env);result=self.call('apply')
        self.assertNotEqual(result.returncode,0)
        self.assertEqual(self.env.read_bytes(),ORIGINAL)
        self.assertEqual(metadata(self.env),before)
        self.assertEqual(len([a for a in self.calls() if 'up' in a]),0)

    def test_restore_refuses_retag_before_environment_or_gateway_write(self):
        self.assertEqual(self.call('apply').returncode,0)
        before=metadata(self.env);data=self.env.read_bytes()
        path=self.root/'docker-state.json';state=json.loads(path.read_text());state['tag_image_id']='sha256:retagged-fixture';path.write_text(json.dumps(state))
        result=self.call('restore')
        self.assertNotEqual(result.returncode,0)
        self.assertEqual(self.env.read_bytes(),data)
        self.assertEqual(metadata(self.env),before)
        self.assertEqual(len([a for a in self.calls() if 'up' in a]),1)

    def test_retag_between_environment_write_and_up_preserves_unknown_without_up(self):
        path=self.root/'docker-state.json';state=json.loads(path.read_text());state['retag_on_config']=2;path.write_text(json.dumps(state))
        result=self.call('apply')
        self.assertNotEqual(result.returncode,0)
        self.assertEqual(self.receipt()['phase'],'applying')
        self.assertEqual(len([a for a in self.calls() if 'up' in a]),0)

    def test_linux_original_source_profile_accepts_only_exact_foreign_deploy_owner(self):
        required = os.environ.get("HENUKIT_REQUIRE_FOREIGN_OWNER") == "1"
        if sys.platform != "linux" or os.geteuid() != 0:
            message = "Notice foreign-owner gate requires Linux/root; this environment cannot prove the original deploy-owner profile"
            if required:self.fail(message)
            self.skipTest(message)
        # This is the OS filesystem/NSS seam, not a production path or user.
        # The inode really has a uid/gid different from the root test executor.
        spec = importlib.util.spec_from_file_location("notice_url_policy",SOURCE)
        module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)
        from types import SimpleNamespace
        owner,group = 12567,12568
        try:
            os.chown(self.source_lock,owner,group)
        except PermissionError as error:
            message = f"Notice foreign-owner fixture chown unavailable: {error.errno}; Linux/root gate remains unverified"
            if required:self.fail(message)
            self.skipTest(message)
        self.source_lock.chmod(0o640)
        directory = self.root/"sticky-lock-parent"; directory.mkdir(mode=0o1777); directory.chmod(0o1777)
        physical = self.source_lock
        class CanonicalSourceFixture:
            parent = directory
            def __eq__(self,other):return other == Path("/run/lock/henukit-deploy.lock")
            def lstat(self):return physical.lstat()
        with patch.object(module.pwd,"getpwnam",return_value=SimpleNamespace(pw_uid=owner,pw_gid=group)):
            info = module.original_source_lock(CanonicalSourceFixture(),production=True)
            self.assertEqual(info.st_uid,owner)
            for bad_uid,bad_gid in ((0,group),(owner,0)):
                os.chown(physical,bad_uid,bad_gid)
                with self.assertRaises(RuntimeError):
                    module.original_source_lock(CanonicalSourceFixture(),production=True)
            os.chown(physical,owner,group); physical.chmod(0o600)
            with self.assertRaises(RuntimeError):
                module.original_source_lock(CanonicalSourceFixture(),production=True)

if __name__ == "__main__":
    unittest.main()
