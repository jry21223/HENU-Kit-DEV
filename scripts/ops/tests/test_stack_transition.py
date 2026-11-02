"""The one-time mixed-stack adoption is exercised through its public CLI."""

from pathlib import Path
import copy
import gzip
import hashlib
import io
import json
import os
import re
import subprocess
import sys
import unittest
import shutil
import tarfile

import test_scoped_food_release as food
import test_scoped_portal_membership_checkpoint as checkpoint
import test_scoped_practice_release as practice_fixture

IMAGES = (
    "platform-core",
    "platform-smtp-provider",
    "platform-mail-worker",
    "portal-api",
    "account-portfolio",
    "notice",
    "notice-worker",
    "food",
    "food-mcp",
    "career-mcp",
    "career-opportunities",
    "library",
    "portal-gateway",
    "quizcraft",
    "portal",
    "portal-summary",
    "console-gateway",
    "console",
)
SCOPE = (*IMAGES, "nginx")
MATERIALS_SHA = "9" * 40
FULL_DEPLOYED_AT = "2026-10-18T05:05:00Z"
FAILED_SHA = "c97d7e93db5b9f12b137e610b7e22bf6b911392e"
FAILED_HELPER = "007f55b1b852d8276ad18b8f9fa4f0576dcf368c806a0dc7825eb13efee92cbd"

FAKE = r"""#!/usr/bin/env python3
import copy,hashlib,json,os,pathlib,sys
root=pathlib.Path(os.environ['FIXTURE']); path=root/'fake-state.json'; state=json.loads(path.read_text());args=sys.argv[1:];name=pathlib.Path(sys.argv[0]).name
with (root/'calls.jsonl').open('a') as f:f.write(json.dumps([name,args])+'\n')
fixture=json.loads((root/'transition-fixture.json').read_text())
def save():path.write_text(json.dumps(state))
def out(v):print(json.dumps(v))
def literal(v):
 if isinstance(v,str):return v.replace('$$','$')
 if isinstance(v,list):return [literal(x) for x in v]
 if isinstance(v,dict):return {k:literal(x) for k,x in v.items()}
 return v
def config():
 source=pathlib.Path(args[args.index('-f')+1])
 if source.name.endswith('.compose.json'):return literal(json.loads(source.read_text()))
 result=copy.deepcopy(fixture['base']);sha=os.environ['RELEASE_SHA']
 for s,c in result['services'].items():
  if s not in ['nginx','postgres','redis','getwork-mcp-relay']:c['image']='henukit-'+s+':'+sha
  for k in list(c.get('environment',{})):
   if k.startswith('QUIZCRAFT_CONSOLE_'):del c['environment'][k]
  if s in ['quizcraft','console-gateway']:
   keys=['ENABLED','CLIENT_ID']+(['API_URL','KEY_ID','SECRET'] if s=='console-gateway' else ['KEYS_JSON'])
   c['environment'].update({'QUIZCRAFT_CONSOLE_'+k:os.environ['QUIZCRAFT_CONSOLE_'+k] for k in keys if 'QUIZCRAFT_CONSOLE_'+k in os.environ})
 if sha==fixture.get('candidate_sha','b'*40):
  result['services'].pop('getwork-mcp-relay',None)
  result['services']['career-opportunities']['environment']['CAREER_GETWORK_ENABLED']=os.environ.get('CAREER_GETWORK_ENABLED','1')
  result['services']['nginx']['volumes'][0]['source']=str(root/'opt/henukit-releases'/sha/'infra/nginx/henukit.conf.example')
  if os.environ.get('UNKNOWN_SCOPE'):result['services']['unexpected']={}
  if os.environ.get('BAD_DATA_ROUTE'):result['services']['platform-core']['environment']['PLATFORM_CORE_DATABASE_URL']='postgres://henukit:password@postgres:5432/unbacked_database?sslmode=disable'
 if fixture.get('strict_metadata'):
  for service in ['portal','portal-summary']:result['services'][service]['environment'].update(PORTAL_VERSION=sha,PORTAL_DEPLOYED_AT=os.environ['PORTAL_DEPLOYED_AT'])
 return result
def config_hash(service,spec):
 suffix=''
 if fixture.get('strict_metadata') and service in ['portal','portal-summary']:suffix='-'+spec['environment']['PORTAL_VERSION']+'-'+spec['environment']['PORTAL_DEPLOYED_AT']
 return 'config-'+service+suffix
if name=='gh':
 if args[:2]==['run','list']:out([{'databaseId':fixture.get('run_id',123),'attempt':state.get('attempt',1)}])
 elif args[:2]==['run','view']:out({'headSha':fixture.get('candidate_sha','b'*40),'headBranch':'main','event':'push','status':'completed','conclusion':'success','workflowName':'Build HENU Kit release artifacts','attempt':state.get('attempt',1)})
 elif args[0]=='api':
  if '/issues/166' in args[1]:print('open' if os.environ.get('BLOCKER_OPEN') else 'closed')
  elif '/actions/runs/456/attempts/2' in args[1]:out({'id':456,'run_attempt':2,'head_sha':'9'*40,'head_branch':'main','event':'push','status':'completed','conclusion':'failure' if os.environ.get('B_ATTEMPT_FAILED') else 'success','path':'.github/workflows/deploy-henukit.yml'})
  elif '/actions/runs/41369983145/attempts/1' in args[1]:out({'id':41369983145,'run_attempt':1,'head_sha':'c97d7e93db5b9f12b137e610b7e22bf6b911392e','head_branch':'main','event':'push','status':'completed','conclusion':'failure' if os.environ.get('OLD_ATTEMPT_FAILED') else 'success','path':'.github/workflows/deploy-henukit.yml'})
  elif '/actions/runs/' in args[1]:
   parts=args[1].split('/');out({'id':int(parts[-3]),'run_attempt':int(parts[-1]),'head_sha':'b'*40,'head_branch':'main','event':'push','status':'completed','conclusion':'success','path':'.github/workflows/deploy-henukit.yml'})
  elif '/compare/' in args[1]:
   old=args[1].split('/compare/')[1].split('...')[0];out({'status':'ahead','behind_by':0,'base_commit':{'sha':old},'merge_base_commit':{'sha':old}})
  elif '/git/commits/' in args[1]:print('d'*40)
  else:print(state.get('source_sha','c'*40 if os.environ.get('STALE_SOURCE') else fixture.get('candidate_sha','b'*40)))
 elif args[:2]==['attestation','verify']:
  if args[args.index('--source-digest')+1]=='9'*40:
   value=json.loads(pathlib.Path(args[args.index('--bundle')+1]).read_text())
   if value.get('fixture_sha')!='9'*40:sys.exit(2)
   if os.environ.get('B_PROOF_DRIFT_DURING_VERIFY'):pathlib.Path(args[args.index('--bundle')+1]).write_text('{"fixture_sha":"swapped after verification"}')
   out([{'verificationResult':{'signature':{'certificate':{'runInvocationURI':'https://github.com/jry21223/HENU-Kit-DEV/actions/runs/456/attempts/2','sourceRepositoryDigest':'9'*40,'buildConfigDigest':'9'*40,'sourceRepositoryRef':'refs/heads/main','runnerEnvironment':'github-hosted','buildConfigURI':'https://github.com/jry21223/HENU-Kit-DEV/.github/workflows/deploy-henukit.yml@refs/heads/main'}}}}])
  elif args[args.index('--source-digest')+1]=='c97d7e93db5b9f12b137e610b7e22bf6b911392e':
   if os.environ.get('BAD_OLD_PROOF'):sys.exit(2)
   sha=args[args.index('--source-digest')+1]
   out([{'verificationResult':{'signature':{'certificate':{'runInvocationURI':'https://github.com/jry21223/HENU-Kit-DEV/actions/runs/41369983145/attempts/'+os.environ.get('OLD_PROOF_ATTEMPT','1'),'sourceRepositoryDigest':sha,'buildConfigDigest':sha,'sourceRepositoryRef':'refs/heads/main','runnerEnvironment':'github-hosted','buildConfigURI':'https://github.com/jry21223/HENU-Kit-DEV/.github/workflows/deploy-henukit.yml@refs/heads/main'}}}}])
  else:
   out([{'verificationResult':{'signature':{'certificate':{'runInvocationURI':'https://github.com/jry21223/HENU-Kit-DEV/actions/runs/'+os.environ.get('NEW_PROOF_RUN','123')+'/attempts/'+os.environ.get('NEW_PROOF_ATTEMPT',str(state.get('attempt',1))),'sourceRepositoryDigest':'b'*40,'buildConfigDigest':'b'*40,'sourceRepositoryRef':'refs/heads/main','runnerEnvironment':'github-hosted','buildConfigURI':'https://github.com/jry21223/HENU-Kit-DEV/.github/workflows/deploy-henukit.yml@refs/heads/main'}}}}])
 else:sys.exit(92)
elif name=='systemctl':
 if args[0]=='is-active':sys.exit(3)
 elif args[0]=='is-enabled':print(state.get('materials_path','disabled'));sys.exit(0 if state.get('materials_path')=='enabled' else 1)
 elif args[0]=='show':
  prop=args[args.index('-p')+1];unit=args[-1]
  print('not-found' if unit=='henukit-deploy-webhook.service' else 'loaded' if prop=='LoadState' else ('active' if state.get('runner_started') else 'inactive') if unit=='henukit-materials-runner.service' else 'active')
 elif args[0] in ['enable','disable']:
  state['materials_path']='enabled' if args[0]=='enable' else 'disabled'
  if args[0]=='enable' and os.environ.get('START_RUNNER_ON_ENABLE'):state['runner_started']=True
  save()
elif name=='curl':
 url=args[-1];code=200;body={}
 if '/api/v1/career/searches' in url and url.startswith('http://'):
  assert args[args.index('--noproxy')+1]=='*'
  received=sys.stdin.read();assert 'X-Signature:' in received and 'X-Actor-User-Id:' in received
  code=503;body={'error':{'code':'JOB_SOURCE_DISABLED'}}
 elif '/workshop/catalog' in url:
  if url=='https://console.henukit.cn/api/v1/quizcraft/workshop/catalog':code=int(os.environ.get('CONSOLE_AUTH_STATUS','401'))
  elif url.startswith('http://') and url.endswith(':10089/api/v1/console/workshop/catalog'):
   assert args[args.index('--noproxy')+1]=='*'
   code=int(os.environ.get('OWNER_AUTH_STATUS','401'))
  else:code=404
 elif '/api/v1/account/summary' in url or '/api/v1/bot/' in url:code=401
 elif '/quiz' in url or '/study-api/' in url:code=404
 if os.environ.get('FAIL_SMOKE') and state.get('deployed') and '/api/v1/library/materials' in url:code=503
 pathlib.Path(args[args.index('--output')+1]).write_text(json.dumps(body));print(code,end='')
elif name=='ssh':
 assert '--execute' not in ' '.join(args)
 if args[-1]=='-s':sys.stdin.buffer.read();print('gateway_ready')
 elif args[-1].startswith('mktemp'):print('/tmp/henukit-transition-check.abcdefgh')
 elif args[-1].startswith('tar '):sys.stdin.buffer.read()
 elif '--check' in args[-1] and os.environ.get('DELAY_GATE_METADATA'):
  import time
  time.sleep(1.2)
elif name=='docker':
 if args[:2]==['ps','-aq']:print('\n'.join(state['containers']))
 elif args[0]=='inspect':out([next(c for key,c in state['containers'].items() if n in (key,c['Id'])) for n in args[1:]])
 elif args[:2]==['image','inspect']:out([{'Id':state['images'][args[2]],'Architecture':'amd64','Os':'linux'}])
 elif args[0]=='load':
  if os.environ.get('DRIFT_DURING_LOAD')=='main':state['source_sha']='c'*40;save()
  if os.environ.get('DRIFT_DURING_LOAD')=='attempt':state['attempt']=2;save()
  if os.environ.get('DRIFT_ENV_DURING_LOAD'):
   envfile=root/'opt/henukit/.env.henukit'
   if os.environ.get('DRIFT_ENV_DURING_LOAD')=='timestamp':envfile.write_text(envfile.read_text().replace('PORTAL_DEPLOYED_AT=2026-10-18T05:05:00Z','PORTAL_DEPLOYED_AT=2026-10-19T05:05:00Z'))
   elif 'FOREIGN_CHANGE=1' not in envfile.read_text():envfile.write_text(envfile.read_text()+'FOREIGN_CHANGE=1\n')
  print('loaded')
 elif args[0]=='compose':
  if 'config' in args and '--hash' in args:print(args[-1]+' '+config_hash(args[-1],config()['services'][args[-1]]))
  elif 'config' in args:out(config())
  elif 'up' in args:
   selected=args[args.index('--no-deps')+1:]
   expected=fixture['stopped'] if '--no-start' in args else [s for s in fixture['scope'] if s not in fixture['stopped']] if 'mixed.compose.json' in args[args.index('-f')+1] else fixture['scope']
   assert sorted(selected)==sorted(expected)
   cfg=config();state['deployed']='candidate.compose.json' in args[args.index('-f')+1]
   for s in selected:
    c=state['containers']['henukit-'+s+'-1'];spec=cfg['services'][s]
    c['Id']='transition-'+s+('-new' if state['deployed'] else '-old');c['Image']=state['images'][spec['image']];c['Config']['Image']=spec['image'];c['Config']['Env']=[k+'='+str(v) for k,v in spec.get('environment',{}).items()]
    if s=='quizcraft':
     old_hosts=pathlib.Path(c['HostsPath']);c['Id']=hashlib.sha256(c['Id'].encode()).hexdigest()
     hosts=root/'var/lib/docker/containers'/c['Id']/'hosts';hosts.parent.mkdir(parents=True,exist_ok=True);hosts.write_bytes(old_hosts.read_bytes());c['HostsPath']=str(hosts)
    c['Config']['Labels']['com.docker.compose.config-hash']=config_hash(s,spec)
    c['Mounts']=[{'Type':'bind','Destination':m['target'],'Source':m['source'],'RW':not m.get('read_only',False)} for m in spec.get('volumes',[]) if m.get('type')=='bind']
    c['State'].update(Running='--no-start' not in args,Status='created' if '--no-start' in args else 'running',Health={'Status':'healthy'})
   save()
  elif 'stop' in args:
   for s in args[args.index('stop')+1:]:state['containers']['henukit-'+s+'-1']['State'].update(Running=False,Status='exited')
   save()
  else:sys.exit(93)
 elif args[0]=='exec':
  if any('pg_dump' in a for a in args):print('database archive')
  elif any('pg_restore' in a for a in args):sys.stdin.buffer.read()
  elif any('psql' in a for a in args):
   if any('selected_role' in a for a in args):out({'roles':1,'permissions':8,'granted':8,'audited':True})
   else:print('1')
 else:sys.exit(94)
__PG_HANDLERS__
else:sys.exit(95)
"""


SOURCE = Path(__file__).resolve().parents[1] / "deploy-henukit-stack-transition.py"


class StackTransitionCLI(unittest.TestCase):
    def test_independent_materials_selection_requires_all_three_arguments(self):
        result = subprocess.run(
            [
                sys.executable,
                str(SOURCE),
                "--sha",
                food.NEW,
                "--run-id",
                "123",
                "--proof-dir",
                "/missing",
                "--acknowledge-getwork-disabled",
                "--materials-baseline-sha",
                MATERIALS_SHA,
                "--preflight",
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 2)
        self.assertIn(
            "requires SHA, runtime and proof directory together", result.stderr
        )

    def test_explicit_disabled_source_acknowledgement_is_required(self):
        result = subprocess.run(
            [
                sys.executable,
                str(SOURCE),
                "--sha",
                "b" * 40,
                "--run-id",
                "123",
                "--proof-dir",
                "/missing",
                "--preflight",
            ],
            capture_output=True,
            text=True,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("--acknowledge-getwork-disabled", result.stderr)


class StackTransitionCheckpoint(checkpoint.PortalMembershipCLI):
    def repack(self):
        manifest = self.proof / ("henukit-getwork-actions-" + food.NEW + ".manifest")
        if manifest.exists():
            manifest.unlink()
        super().repack()
        image = self.artifacts / ("henukit-getwork-mcp-" + food.NEW + ".docker.tar.gz")
        image.write_bytes(b"unused getWork image carried by the v1 proof contract")
        Path(str(image) + ".sha256").write_text(
            self.digest(image) + "  " + image.name + "\n"
        )
        manifest.unlink()
        result = subprocess.run(
            [
                "bash",
                str(SOURCE.with_name("create-getwork-actions-manifest.sh")),
                "--sha",
                food.NEW,
                "--artifact-dir",
                str(self.artifacts),
                "--output",
                str(manifest),
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)

    def setUp(self):
        super().setUp()
        shutil.copy(SOURCE, self.runtime / "bin" / SOURCE.name)
        shutil.copy(
            SOURCE.with_name("deploy-henukit-practice-scoped.py"),
            self.runtime / "bin/deploy-henukit-practice-scoped.py",
        )
        self.repack()

    def transition(self, mode, extra=None, arguments=()):
        return subprocess.run(
            [
                sys.executable,
                str(self.runtime / "bin" / SOURCE.name),
                "--sha",
                food.NEW,
                "--run-id",
                "123",
                "--proof-dir",
                str(self.proof),
                "--test-root",
                str(self.root),
                "--acknowledge-getwork-disabled",
                mode,
                *arguments,
            ],
            env={**self.env, **(extra or {})},
            capture_output=True,
            text=True,
        )

    def test_transition_requires_actual_active_v3_without_online_mutation(self):
        before = (self.root / "fake-state.json").read_bytes()
        env = self.envfile.read_bytes()
        result = self.transition("--preflight")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("active Portal v3", result.stderr)
        self.assertEqual(before, (self.root / "fake-state.json").read_bytes())
        self.assertEqual(env, self.envfile.read_bytes())

    def full_fixture(self):
        result = self.run_cli("--execute")
        self.assertEqual(result.returncode, 0, result.stderr)
        current_record = self.record.parent.with_name("e" * 40)
        self.record.parent.rename(current_record)
        current_runtime = self.root / "retained-portal-after-membership"
        shutil.copytree(self.runtime, current_runtime)
        (current_runtime / "RELEASE_SHA").write_text("e" * 40 + "\n")
        record = current_record / "receipt.json"
        current = json.loads(record.read_text())
        current.update(sha="e" * 40, runtime=str(current_runtime))
        record.write_text(json.dumps(current))
        self.marker.unlink()
        self.marker.symlink_to(current_record)
        state_file = self.root / "fake-state.json"
        state = json.loads(state_file.read_text())
        item = state["containers"]["henukit-portal-1"]
        item["Config"]["Image"] = "henukit-portal:" + "e" * 40
        state["images"][item["Config"]["Image"]] = item["Image"]
        # Reuse the real public host-PG16 command fixture, including namespace
        # inode checks, exported snapshots, and isolated restored row counts.
        pg = practice_fixture.PracticeCLI()
        pg.setUp()
        self.addCleanup(pg.doCleanups)
        pg_state = json.loads((pg.root / "fake-state.json").read_text())
        quiz = state["containers"]["henukit-quizcraft-1"]
        for key in ("Id", "HostsPath", "HostConfig", "NetworkSettings"):
            quiz[key] = copy.deepcopy(
                pg_state["containers"]["henukit-quizcraft-1"][key]
            )
        quiz["State"].update(Pid=4242, StartedAt="2026-10-01T00:00:00Z")
        for dirname in ("proc", "run", "var/lib/docker"):
            shutil.copytree(pg.root / dirname, self.root / dirname, dirs_exist_ok=True)
        # copytree does not retain the namespace/sandbox hard link.
        sandbox = self.root / "run/docker/netns/fixture-quizcraft"
        sandbox.unlink()
        os.link(self.root / "proc/4242/ns/net", sandbox)
        quiz["HostsPath"] = str(
            self.root / "var/lib/docker/containers" / quiz["Id"] / "hosts"
        )
        quiz["NetworkSettings"]["SandboxKey"] = str(sandbox)
        base = {"services": {}}
        for service in (*SCOPE, "postgres", "redis", "getwork-mcp-relay"):
            name = "henukit-" + service + "-1"
            if name not in state["containers"]:
                c = copy.deepcopy(state["containers"]["henukit-library-1"])
                c.update(Name="/" + name, Id="original-" + service)
                c["Config"]["Image"] = "henukit-" + service + ":" + food.OLD
                c["Config"]["Labels"]["com.docker.compose.config-hash"] = (
                    "config-" + service
                )
                c["Image"] = "sha256:" + hashlib.sha256(service.encode()).hexdigest()
                state["containers"][name] = c
                state["images"][c["Config"]["Image"]] = c["Image"]
            c = state["containers"][name]
            environment = dict(v.split("=", 1) for v in c["Config"]["Env"])
            base["services"][service] = {
                "image": c["Config"]["Image"],
                "environment": environment,
            }
        databases = {
            "platform-core": ("PLATFORM_CORE_DATABASE_URL", "platform"),
            "account-portfolio": (
                "ACCOUNT_PORTFOLIO_DATABASE_URL",
                "account_portfolio",
            ),
            "notice": ("NOTICE_DATABASE_URL", "notice"),
            "food": ("FOOD_DATABASE_URL", "food"),
            "library": ("LIBRARY_DATABASE_URL", "library"),
            "career-opportunities": ("CAREER_DATABASE_URL", "career"),
            "portal-api": ("STUDY_DATABASE_URL", "study"),
        }
        for service, (key, database) in databases.items():
            base["services"][service]["environment"][key] = (
                "postgres://henukit:password@postgres:5432/"
                + database
                + "?sslmode=disable"
            )
        base["services"]["postgres"]["environment"].update(
            POSTGRES_USER="henukit", POSTGRES_PASSWORD="password"
        )
        base["services"]["quizcraft"]["environment"]["DATABASE_URL"] = (
            "postgres://quizcraft_v2:password@host.docker.internal:5432/quizcraft_v2?sslmode=disable"
        )
        base["services"]["career-opportunities"]["environment"].update(
            CAREER_SERVICE_CLIENT_ID="portal-gateway-career",
            CAREER_SERVICE_KEY_ID="career-key-1",
            CAREER_SERVICE_SECRET="career-secret-32-characters-minimum",
        )
        state["containers"]["henukit-career-opportunities-1"]["NetworkSettings"] = {
            "Networks": {"henukit_default": {"IPAddress": "172.18.0.9"}}
        }
        for service, config in base["services"].items():
            state["containers"]["henukit-" + service + "-1"]["Config"]["Env"] = [
                k + "=" + str(v) for k, v in config.get("environment", {}).items()
            ]
        member = json.loads(self.membership_record.read_text())
        private = json.loads(
            (self.membership_record.parent / "after.compose.json").read_text()
        )
        private["services"]["account-portfolio"]["environment"].update(
            {
                "ACCOUNT_PORTFOLIO_DATABASE_URL": base["services"]["account-portfolio"][
                    "environment"
                ]["ACCOUNT_PORTFOLIO_DATABASE_URL"]
            }
        )
        private_path = self.membership_record.parent / "after.compose.json"
        private_path.write_text(json.dumps(private))
        member["configuration_hashes"]["after.compose.json"] = self.digest(private_path)
        self.membership_record.write_text(json.dumps(member))
        base["services"]["nginx"] = copy.deepcopy(private["services"]["nginx"])
        base["services"]["nginx"]["environment"] = dict(
            v.split("=", 1)
            for v in state["containers"]["henukit-nginx-1"]["Config"]["Env"]
        )
        route = self.runtime / "infra/nginx/henukit.conf.example"
        route.parent.mkdir(parents=True, exist_ok=True)
        route.write_bytes((self.membership_record.parent / "nginx.conf").read_bytes())
        prior_record = (
            self.root
            / "var/lib/henukit-scoped-releases/practice-console"
            / ("c" * 40)
            / "receipt.json"
        )
        application_file = prior_record.parent / "quizcraft-console-application.json"
        application_file.write_text(
            json.dumps(
                {
                    "client_id": "console-gateway-quizcraft-management",
                    "key_id": "quizcraft-console-management-v1",
                    "secret": "s" * 64,
                }
            )
        )
        application_file.chmod(0o600)
        prior = json.loads(prior_record.read_text())
        prior["application_secret_sha256"] = self.digest(application_file)
        prior_record.write_text(json.dumps(prior))
        app = {
            "QUIZCRAFT_CONSOLE_ENABLED": "1",
            "QUIZCRAFT_CONSOLE_CLIENT_ID": "console-gateway-quizcraft-management",
            "QUIZCRAFT_CONSOLE_KEY_ID": "quizcraft-console-management-v1",
            "QUIZCRAFT_CONSOLE_API_URL": "http://quizcraft:10089",
            "QUIZCRAFT_CONSOLE_SECRET": "s" * 64,
            "QUIZCRAFT_CONSOLE_KEYS_JSON": json.dumps(
                {"quizcraft-console-management-v1": "s" * 64}, separators=(",", ":")
            ),
        }
        for service, names in {
            "console-gateway": ("ENABLED", "CLIENT_ID", "API_URL", "KEY_ID", "SECRET"),
            "quizcraft": ("ENABLED", "CLIENT_ID", "KEYS_JSON"),
        }.items():
            for suffix in names:
                key = "QUIZCRAFT_CONSOLE_" + suffix
                base["services"][service]["environment"][key] = app[key]
                state["containers"]["henukit-" + service + "-1"]["Config"][
                    "Env"
                ].append(key + "=" + app[key])

        # The minimal inherited fixture omits unrelated full services. Add those
        # immutable historical snapshot facts before the v3 chain is sealed.
        def snapshot(c):
            return {
                "id": c["Id"],
                "image": c["Image"],
                "tag": c["Config"]["Image"],
                "status": c["State"]["Status"],
                "running": c["State"]["Running"],
                "restarts": c["RestartCount"],
                "started_at": c["State"].get("StartedAt", ""),
                "finished_at": c["State"].get("FinishedAt", ""),
                "exit_code": c["State"].get("ExitCode", 0),
                "pid": c["State"].get("Pid", 0),
            }

        paths = [self.portal_record, self.membership_record, record]
        for path in paths:
            data = json.loads(path.read_text())
            for name, c in state["containers"].items():
                if name != "henukit-portal-1" and name not in data.get(
                    "selected_checkpoint", {}
                ):
                    data.setdefault("unselected", {})[name] = snapshot(c)
            if path == self.portal_record:
                data["unselected"].pop("henukit-account-portfolio-1", None)
                data["unselected"].pop("henukit-nginx-1", None)
            if "predecessor_record" in data:
                data["predecessor_receipt_sha256"] = self.digest(
                    Path(data["predecessor_record"])
                )
            if "membership_checkpoint" in data:
                data["membership_checkpoint"]["sha256"] = self.digest(
                    self.membership_record
                )
            path.write_text(json.dumps(data))
        state["deployed"] = False
        state_file.write_text(json.dumps(state))
        for service in IMAGES:
            config = json.dumps(
                {"os": "linux", "architecture": "amd64", "service": service}
            ).encode()
            state["images"]["henukit-" + service + ":" + food.NEW] = (
                "sha256:" + hashlib.sha256(config).hexdigest()
            )
            archive = self.artifacts / (
                "henukit-" + service + "-" + food.NEW + ".docker.tar.gz"
            )
            with tarfile.open(archive, "w:gz") as stream:
                for name, content in [
                    ("config.json", config),
                    (
                        "manifest.json",
                        json.dumps(
                            [
                                {
                                    "Config": "config.json",
                                    "RepoTags": ["henukit-" + service + ":" + food.NEW],
                                    "Layers": [],
                                }
                            ]
                        ).encode(),
                    ),
                ]:
                    info = tarfile.TarInfo(name)
                    info.size = len(content)
                    stream.addfile(info, io.BytesIO(content))
            Path(str(archive) + ".sha256").write_text(
                self.digest(archive) + "  " + archive.name + "\n"
            )
        state_file.write_text(json.dumps(state))
        roots = {
            self.runtime,
            self.root / "opt/henukit-releases" / food.OLD,
            Path(prior["runtime"]),
            Path(member["runtime"]),
        }
        for root in roots:
            for owner in (
                "platform-core",
                "account-portfolio",
                "notice",
                "food",
                "library",
                "career",
                "portal",
                "quizcraft",
            ):
                folder = root / "migrations" / owner
                folder.mkdir(parents=True, exist_ok=True)
                for old in folder.iterdir():
                    old.unlink()
                (folder / "001.up.sql").write_text("existing " + owner + " schema")
        installed = self.root / "usr/local/bin/henukit-deploy-webhook"
        installed.parent.mkdir(parents=True)
        installed.write_bytes(b"previous materials runtime")
        installed.chmod(0o755)
        for root, content in (
            (
                self.root / "opt/henukit-releases" / food.OLD,
                b"previous materials runtime",
            ),
            (self.runtime, b"candidate materials runtime"),
        ):
            target = root / "materials-runtime/bin/henukit-deploy-webhook"
            target.parent.mkdir(parents=True)
            target.write_bytes(content)
        installer = self.runtime / "materials-runtime/install.sh"
        installer.write_text(
            """#!/usr/bin/env python3
import os,pathlib,shutil
table="""
            + "'''"
            + """declare -a install_records=(
  "bin/henukit-deploy-webhook|/usr/local/bin/henukit-deploy-webhook|0755"
)"""
            + "'''"
            + """
root=pathlib.Path(os.environ['FIXTURE']);sha='b'*40
target=root/'usr/local/bin/henukit-deploy-webhook'
backup=root/'opt/henukit-materials/runtime-backups'/sha/'usr/local/bin/henukit-deploy-webhook'
backup.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(target,backup)
target.write_bytes((pathlib.Path(__file__).parent/'bin/henukit-deploy-webhook').read_bytes())
if os.environ.get('INTERRUPT_INSTALL'):
 import signal
 os.kill(os.getppid(),signal.SIGKILL)
"""
        )
        installer.chmod(0o755)
        payload = installer.parent
        (payload / "SHA256SUMS").write_text(
            "".join(
                self.digest(p) + "  " + str(p.relative_to(payload)) + "\n"
                for p in sorted(payload.rglob("*"))
                if p.is_file()
            )
        )
        config = self.root / "etc/henukit-deploy/materials-activate.env"
        config.parent.mkdir(parents=True)
        config.write_text("MODERN=1\n")
        config.chmod(0o600)
        (self.root / "opt/henukit-materials").mkdir(exist_ok=True)
        self.envfile.write_text(
            self.envfile.read_text()
            + "\n"
            + "\n".join(
                (
                    "PORTAL_API_MODE=live",
                    "PORTAL_DEPLOYED_AT=" + FULL_DEPLOYED_AT,
                    "NEXT_PUBLIC_PORTAL_ALLOW_MOCK=0",
                    "ACCOUNT_PORTFOLIO_EASYPAY_ENABLED=1",
                    "ACCOUNT_PORTFOLIO_EASYPAY_PID=henukit",
                    "ACCOUNT_PORTFOLIO_EASYPAY_KEY=" + "e" * 64,
                    "ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL=https://metaview.top/epay",
                    "ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL=https://henukit.cn/api/v1/payment-providers/easypay/notifications",
                    "ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL=https://henukit.cn/account/membership",
                    "CAREER_AI_BASE_URL=https://llm.provider.invalid/v1",
                    "CAREER_AI_API_KEY=" + "a" * 64,
                    "CAREER_AI_MODEL=production-model",
                    "PLATFORM_CORE_CAREER_DIGEST_CLIENT_ID=career-digest",
                    "PLATFORM_CORE_CAREER_DIGEST_KEY_ID=career-digest-key",
                    "PLATFORM_CORE_CAREER_DIGEST_SECRET=" + "d" * 64,
                )
            )
            + "\n"
        )
        member = json.loads(self.membership_record.read_text())
        member["environment_sha256"] = self.digest(self.envfile)
        (self.membership_record.parent / "configuration.backup").write_bytes(
            self.envfile.read_bytes()
        )
        self.membership_record.write_text(json.dumps(member))
        current = json.loads(record.read_text())
        current["membership_checkpoint"]["sha256"] = self.digest(self.membership_record)
        record.write_text(json.dumps(current))
        policy = self.root / "etc/henukit/actions-watch.env"
        policy.write_text("HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE=operator\n")
        policy.chmod(0o600)
        gates = self.runtime / "release-gates"
        gates.mkdir(exist_ok=True)
        (gates / "account-production-boundary.env").write_text(
            "\n".join(
                (
                    "release_sha=" + food.NEW,
                    "status=pass",
                    "account_console_mock_sources=absent",
                    "account_transitive_mock_sources=absent",
                    "account_payment_provider=easypay_or_disabled",
                    "portal_require_gateway=1",
                    "portal_allow_mock=0",
                    "portal_api_default_mode=live",
                )
            )
            + "\n"
        )
        oauth = (
            "format=henukit-oauth-continuation-gate-v1\nrelease_sha="
            + food.NEW
            + "\nsource_tree="
            + "d" * 40
            + "\nresult=pass\n"
        )
        (gates / "oauth-continuation.env").write_text(oauth)
        (self.artifacts / ("oauth-continuation-" + food.NEW + ".env")).write_text(oauth)
        for root in (self.runtime, self.root / "opt/henukit-releases" / food.OLD):
            for name in (
                "bin/deploy-epay-gateway-patches.sh",
                "infra/epay-gateway/patches/0001-henukit-query-and-notify-outbox.patch",
                "infra/epay-gateway/patches/0002-henukit-close-refund-and-response-verification.patch",
                "infra/epay-gateway/patches/0003-henukit-private-checkout-handle.patch",
            ):
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                if name == "bin/deploy-epay-gateway-patches.sh":
                    shutil.copy(
                        SOURCE.with_name("deploy-epay-gateway-patches.sh"), path
                    )
                else:
                    path.write_text("existing verified gateway code\n")
        (self.root / "transition-fixture.json").write_text(
            json.dumps(
                {
                    "base": base,
                    "scope": SCOPE,
                    "stopped": ["career-opportunities", "career-mcp"],
                }
            )
        )
        pg_fake = (pg.root / "bin/psql").read_text()
        handlers = pg_fake[
            pg_fake.index("elif name=='nsenter':") : pg_fake.index(
                "elif name=='docker':"
            )
        ]
        for name in (
            "docker",
            "gh",
            "systemctl",
            "curl",
            "ssh",
            "psql",
            "pg_dump",
            "pg_restore",
            "createdb",
            "dropdb",
            "runuser",
            "nsenter",
        ):
            path = self.root / "bin" / name
            path.write_text(FAKE.replace("__PG_HANDLERS__", handlers))
            path.chmod(0o755)
        bash = self.root / "bin/bash"
        bash.write_text('#!/bin/sh\nexec /bin/bash "$@"\n')
        bash.chmod(0o755)
        self.repack()
        self.full_record = record

    def separate_materials_fixture(self):
        """A full application baseline plus a real, coherent 19-target B."""
        self.full_fixture()
        self.materials_runtime = self.root / "opt/henukit-releases" / MATERIALS_SHA
        shutil.copytree(self.runtime, self.materials_runtime)
        (self.materials_runtime / "RELEASE_SHA").write_text(MATERIALS_SHA + "\n")
        text = (
            SOURCE.parents[2]
            / "services/deploy-webhook/deploy/install-materials-runtime.sh"
        ).read_text()
        table = re.search(r"declare -a install_records=\(\n(.*?)\n\)", text, re.S)[1]
        records = [
            re.fullmatch(r'\s*"([^|]+)\|([^|]+)\|([0-7]+)"', line).groups()
            for line in table.splitlines()
        ]
        self.assertEqual(len(records), 19)
        self.materials_targets = {}
        for source, target, mode in records:
            before = ("delivered B: " + source).encode()
            candidate = ("candidate: " + source).encode()
            for runtime, content in (
                (self.materials_runtime, before),
                (self.runtime, candidate),
            ):
                path = runtime / "materials-runtime" / source
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(content)
            live = self.root / target.lstrip("/")
            live.parent.mkdir(parents=True, exist_ok=True)
            live.write_bytes(before)
            live.chmod(int(mode, 8))
            self.materials_targets[live] = (before, int(mode, 8))
        for runtime in (self.materials_runtime, self.runtime):
            installer = runtime / "materials-runtime/install.sh"
            script = (
                "#!/usr/bin/env python3\nimport os,pathlib,shutil\ntable='''declare -a install_records=(\n"
                + table
                + "\n)'''\n"
            )
            if runtime == self.materials_runtime:
                script += "raise SystemExit('historical code must never execute')\n"
            else:
                script += (
                    "records="
                    + repr(records)
                    + "\nroot=pathlib.Path(os.environ['FIXTURE'])\n"
                )
                script += "for source,target,mode in records:\n live=root/target.lstrip('/')\n backup=root/'opt/henukit-materials/runtime-backups'/('b'*40)/target.lstrip('/')\n backup.parent.mkdir(parents=True,exist_ok=True)\n shutil.copy2(live,backup)\n live.write_bytes((pathlib.Path(__file__).parent/source).read_bytes())\n live.chmod(int(mode,8))\n"
                script += "if os.environ.get('INTERRUPT_INSTALL'):\n import signal\n os.kill(os.getppid(),signal.SIGKILL)\n"
            installer.write_text(script)
            installer.chmod(0o755)
            manifest = installer.parent / "SHA256SUMS"
            manifest.unlink()
            manifest.write_text(
                "".join(
                    self.digest(p) + "  " + str(p.relative_to(installer.parent)) + "\n"
                    for p in sorted(installer.parent.rglob("*"))
                    if p.is_file()
                )
            )
        self.materials_proof = self.root / "historical-materials-proof"
        self.materials_proof.mkdir()
        archive = self.materials_proof / (
            "henukit-runtime-" + MATERIALS_SHA + ".tar.gz"
        )
        with tarfile.open(archive, "w:gz") as stream:
            for entry in self.materials_runtime.rglob("*"):
                if entry.is_file():
                    stream.add(
                        entry,
                        arcname="./" + str(entry.relative_to(self.materials_runtime)),
                    )
        Path(str(archive) + ".sha256").write_text(
            self.digest(archive) + "  " + archive.name + "\n"
        )
        image = self.materials_proof / (
            "henukit-getwork-mcp-" + MATERIALS_SHA + ".docker.tar.gz"
        )
        image.write_bytes(b"unused historical getWork image")
        Path(str(image) + ".sha256").write_text(
            self.digest(image) + "  " + image.name + "\n"
        )
        result = subprocess.run(
            [
                "bash",
                str(SOURCE.with_name("create-getwork-actions-manifest.sh")),
                "--sha",
                MATERIALS_SHA,
                "--artifact-dir",
                str(self.materials_proof),
                "--output",
                str(
                    self.materials_proof
                    / ("henukit-getwork-actions-" + MATERIALS_SHA + ".manifest")
                ),
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        (
            self.materials_proof
            / ("henukit-getwork-actions-" + MATERIALS_SHA + ".attestation.json")
        ).write_text(json.dumps({"fixture_sha": MATERIALS_SHA}))
        (self.materials_proof / "trusted_root.jsonl").write_text(
            "historical public trust root\n"
        )
        self.materials_arguments = (
            "--materials-baseline-sha",
            MATERIALS_SHA,
            "--materials-baseline-runtime",
            str(self.materials_runtime),
            "--materials-baseline-proof-dir",
            str(self.materials_proof),
        )
        self.repack()

    def test_explicit_materials_baseline_preserves_independent_delivered_capabilities(
        self,
    ):
        self.separate_materials_fixture()
        before = (self.root / "fake-state.json").read_bytes(), self.envfile.read_bytes()
        rejected = self.transition("--preflight")
        self.assertNotEqual(rejected.returncode, 0)
        result = self.transition("--preflight", arguments=self.materials_arguments)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            before,
            ((self.root / "fake-state.json").read_bytes(), self.envfile.read_bytes()),
        )
        self.assertEqual((self.root / "opt/henukit/current").resolve().name, food.OLD)

    def test_independent_materials_rejects_missing_unverified_or_failed_attempt_proof(
        self,
    ):
        self.separate_materials_fixture()
        path = self.materials_proof / (
            "henukit-getwork-actions-" + MATERIALS_SHA + ".attestation.json"
        )
        original = path.read_bytes()
        before = (self.root / "fake-state.json").read_bytes()
        for fault in ("missing", "unverified", "failed_attempt", "during_verify"):
            with self.subTest(fault):
                if path.exists():
                    path.unlink()
                if fault != "missing":
                    path.write_bytes(
                        original
                        if fault in ("failed_attempt", "during_verify")
                        else b'{"fixture_sha":"wrong"}'
                    )
                result = self.transition(
                    "--execute",
                    {"B_ATTEMPT_FAILED": "1"}
                    if fault == "failed_attempt"
                    else {"B_PROOF_DRIFT_DURING_VERIFY": "1"}
                    if fault == "during_verify"
                    else None,
                    arguments=self.materials_arguments,
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual((self.root / "fake-state.json").read_bytes(), before)
                self.assertFalse(
                    (
                        self.root / "var/lib/henukit-scoped-releases/stack-transition"
                    ).exists()
                )

    def test_independent_materials_rejects_partial_live_target_drift(self):
        self.separate_materials_fixture()
        path = self.root / "usr/local/libexec/henukit/henukit-materials-control"
        path.write_bytes(b"unknown partial install")
        before = (self.root / "fake-state.json").read_bytes()
        result = self.transition("--execute", arguments=self.materials_arguments)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("materials runtime differs", result.stderr)
        self.assertEqual(path.read_bytes(), b"unknown partial install")
        self.assertEqual((self.root / "fake-state.json").read_bytes(), before)
        content, mode = self.materials_targets[path]
        path.write_bytes(content)
        for changed in (mode ^ 0o100, mode | 0o4000, mode | 0o2000):
            with self.subTest(mode=oct(changed)):
                path.chmod(changed)
                result = self.transition(
                    "--execute", arguments=self.materials_arguments
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("mode/owner differs", result.stderr)
                self.assertEqual(path.stat().st_mode & 0o7777, changed)
        path.chmod(mode)

    def test_independent_materials_retained_bytes_drift_after_preflight_is_refused(
        self,
    ):
        self.separate_materials_fixture()
        self.assertEqual(
            self.transition(
                "--preflight", arguments=self.materials_arguments
            ).returncode,
            0,
        )
        path = (
            self.materials_runtime
            / "materials-runtime/libexec/henukit-materials-control"
        )
        path.write_bytes(b"changed retained baseline")
        before = (self.root / "fake-state.json").read_bytes()
        result = self.transition("--execute", arguments=self.materials_arguments)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("runtime bytes differ", result.stderr)
        self.assertEqual((self.root / "fake-state.json").read_bytes(), before)
        for live, (content, mode) in self.materials_targets.items():
            self.assertEqual(live.read_bytes(), content)
            self.assertEqual(live.stat().st_mode & 0o777, mode)

    def test_failed_transition_restores_the_independent_materials_cohort(self):
        self.separate_materials_fixture()
        arguments = list(self.materials_arguments)
        arguments[3] += "/"
        arguments[5] += "/"
        result = self.transition("--execute", {"FAIL_SMOKE": "1"}, arguments=arguments)
        self.assertNotEqual(result.returncode, 0)
        record = (
            self.root
            / "var/lib/henukit-scoped-releases/stack-transition"
            / food.NEW
            / "receipt.json"
        )
        data = json.loads(record.read_text())
        self.assertEqual(data["status"], "rolled_back", result.stderr)
        self.assertEqual(data["baseline"], food.OLD)
        self.assertEqual(data["materials"]["baseline"]["sha"], MATERIALS_SHA)
        self.assertEqual(len(data["materials"]["targets"]), 19)
        for live, (content, mode) in self.materials_targets.items():
            self.assertEqual(live.read_bytes(), content)
            self.assertEqual(live.stat().st_mode & 0o777, mode)
        self.assertEqual((self.root / "opt/henukit/current").resolve().name, food.OLD)
        result = self.transition("--rollback")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "already_rolled_back")
        retained = (
            self.materials_runtime
            / "materials-runtime/libexec/henukit-materials-control"
        )
        saved = retained.read_bytes()
        retained.write_bytes(b"drift after durable receipt")
        result = self.transition("--rollback")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("retained materials baseline runtime changed", result.stderr)
        retained.write_bytes(saved)

    def test_interrupted_independent_materials_refuses_foreign_identity_then_restores_B(
        self,
    ):
        self.separate_materials_fixture()
        result = self.transition(
            "--execute", {"INTERRUPT_INSTALL": "1"}, arguments=self.materials_arguments
        )
        self.assertEqual(result.returncode, -9, result.stderr)
        record = (
            self.root
            / "var/lib/henukit-scoped-releases/stack-transition"
            / food.NEW
            / "receipt.json"
        )
        data = json.loads(record.read_text())
        self.assertEqual(data["status"], "materials_installing")
        baseline = data["materials"]["baseline"]
        self.assertEqual((baseline["run_id"], baseline["attempt"]), (456, 2))
        self.assertEqual(
            baseline["archive_sha256"], self.digest(Path(baseline["archive"]))
        )
        self.assertEqual(
            baseline["payload_manifest_sha256"],
            self.digest(self.materials_runtime / "materials-runtime/SHA256SUMS"),
        )
        live = self.root / "usr/local/libexec/henukit/henukit-materials-control"
        candidate = live.read_bytes()
        before_state = (self.root / "fake-state.json").read_bytes()
        environment = self.envfile.read_bytes()
        backup = (
            Path(data["materials"]["backup"])
            / "usr/local/libexec/henukit/henukit-materials-control"
        )
        proof = self.materials_proof / "trusted_root.jsonl"
        original_proof = proof.read_bytes()
        correct_mode = self.materials_targets[live][1]
        for fault in (
            "foreign_bytes",
            "foreign_mode",
            "foreign_suid",
            "foreign_sgid",
            "backup_mode",
            "backup_suid",
            "backup_sgid",
            "proof",
        ):
            with self.subTest(fault):
                if fault == "foreign_bytes":
                    live.write_bytes(b"independently installed foreign C")
                if fault == "foreign_mode":
                    live.chmod(0o600)
                if fault == "foreign_suid":
                    live.chmod(correct_mode | 0o4000)
                if fault == "foreign_sgid":
                    live.chmod(correct_mode | 0o2000)
                if fault == "backup_mode":
                    backup.chmod(0o600)
                if fault == "backup_suid":
                    backup.chmod(correct_mode | 0o4000)
                if fault == "backup_sgid":
                    backup.chmod(correct_mode | 0o2000)
                if fault == "proof":
                    proof.write_bytes(b"unapproved replacement root")
                changed_identity = (live.read_bytes(), live.stat().st_mode & 0o7777)
                (self.root / "calls.jsonl").unlink()
                result = self.transition("--rollback")
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(
                    (self.root / "fake-state.json").read_bytes(), before_state
                )
                self.assertEqual(self.envfile.read_bytes(), environment)
                self.assertEqual(
                    (live.read_bytes(), live.stat().st_mode & 0o7777), changed_identity
                )
                self.assertFalse(
                    any(
                        name == "docker" and ("up" in args or "stop" in args)
                        for name, args in self.calls()
                    )
                )
                live.write_bytes(candidate)
                live.chmod(0o700)
                backup.chmod(0o700)
                proof.write_bytes(original_proof)
        result = self.transition("--rollback")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "rolled_back")
        for live, (content, mode) in self.materials_targets.items():
            self.assertEqual(live.read_bytes(), content)
            self.assertEqual(live.stat().st_mode & 0o7777, mode)
            self.assertEqual(
                (live.stat().st_uid, live.stat().st_gid), (os.geteuid(), os.getegid())
            )

    def test_complete_preflight_binds_real_mixed_checkpoint_without_mutation(self):
        self.full_fixture()
        before = (
            (self.root / "fake-state.json").read_bytes(),
            self.envfile.read_bytes(),
            self.full_record.read_bytes(),
        )
        result = self.transition("--preflight")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "preflight_passed")
        self.assertEqual(
            before,
            (
                (self.root / "fake-state.json").read_bytes(),
                self.envfile.read_bytes(),
                self.full_record.read_bytes(),
            ),
        )
        self.assertFalse(
            (self.root / "var/lib/henukit-scoped-releases/stack-transition").exists()
        )

    def test_workshop_boundaries_use_console_and_private_owner_before_mutation(self):
        self.full_fixture()
        result = self.transition("--preflight")
        self.assertEqual(result.returncode, 0, result.stderr)
        urls = [args[-1] for name, args in self.calls() if name == "curl"]
        self.assertIn(
            "https://console.henukit.cn/api/v1/quizcraft/workshop/catalog", urls
        )
        self.assertTrue(
            any(
                u.startswith("http://")
                and u.endswith(":10089/api/v1/console/workshop/catalog")
                for u in urls
            )
        )
        self.assertNotIn("https://henukit.cn/api/v1/quizcraft/workshop/catalog", urls)
        for key in ("CONSOLE_AUTH_STATUS", "OWNER_AUTH_STATUS"):
            for status in ("200", "403", "404", "503"):
                with self.subTest(boundary=key, status=status):
                    before = (self.root / "fake-state.json").read_bytes()
                    env = self.envfile.read_bytes()
                    (self.root / "calls.jsonl").write_text("")
                    result = self.transition("--execute", {key: status})
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn("acceptance failed", result.stderr)
                    self.assertEqual(
                        before, (self.root / "fake-state.json").read_bytes()
                    )
                    self.assertEqual(env, self.envfile.read_bytes())
                    self.assertFalse(
                        any(
                            name == "docker" and ("load" in args or "up" in args)
                            for name, args in self.calls()
                        )
                    )

    def failed_predecessor_fixture(self):
        """Execute the authentic c97 helper against the origin-aware HTTP seam."""
        self.full_fixture()
        old_runtime = self.runtime.with_name(FAILED_SHA)
        shutil.copytree(self.runtime, old_runtime)
        (old_runtime / "RELEASE_SHA").write_text(FAILED_SHA + "\n")
        original = gzip.decompress(
            (Path(__file__).parent / "fixtures/stack-transition-c97.py.gz").read_bytes()
        )
        self.assertEqual(hashlib.sha256(original).hexdigest(), FAILED_HELPER)
        (old_runtime / "bin" / SOURCE.name).write_bytes(original)
        for name in (
            "release-gates/account-production-boundary.env",
            "release-gates/oauth-continuation.env",
            "materials-runtime/install.sh",
        ):
            p = old_runtime / name
            p.write_text(
                p.read_text()
                .replace(food.NEW, FAILED_SHA)
                .replace("'b'*40", repr(FAILED_SHA))
            )
        payload = old_runtime / "materials-runtime"
        (payload / "SHA256SUMS").write_text(
            "".join(
                self.digest(p) + "  " + str(p.relative_to(payload)) + "\n"
                for p in sorted(payload.rglob("*"))
                if p.is_file() and p.name != "SHA256SUMS"
            )
        )
        stage = self.artifacts.with_name(FAILED_SHA)
        stage.mkdir()
        for p in self.artifacts.glob("*.docker.tar.gz"):
            new = stage / p.name.replace(food.NEW, FAILED_SHA)
            if "getwork-mcp" in p.name:
                new.write_bytes(p.read_bytes())
            else:
                with tarfile.open(p, "r:gz") as tar:
                    entries = [
                        (m.name, tar.extractfile(m).read()) for m in tar if m.isfile()
                    ]
                with tarfile.open(new, "w:gz") as tar:
                    for name, content in entries:
                        if name == "manifest.json":
                            content = content.replace(
                                food.NEW.encode(), FAILED_SHA.encode()
                            )
                        info = tarfile.TarInfo(name)
                        info.size = len(content)
                        tar.addfile(info, io.BytesIO(content))
            Path(str(new) + ".sha256").write_text(
                self.digest(new) + "  " + new.name + "\n"
            )
        archive = stage / ("henukit-runtime-" + FAILED_SHA + ".tar.gz")
        with tarfile.open(archive, "w:gz") as tar:
            for p in old_runtime.rglob("*"):
                if p.is_file():
                    tar.add(p, arcname="./" + str(p.relative_to(old_runtime)))
        Path(str(archive) + ".sha256").write_text(
            self.digest(archive) + "  " + archive.name + "\n"
        )
        self.old_proof = self.root / "old-proof"
        self.old_proof.mkdir()
        for p in self.proof.iterdir():
            if p.name in ("trusted_root.jsonl",) or p.name.endswith(
                ".attestation.json"
            ):
                shutil.copy(p, self.old_proof / p.name.replace(food.NEW, FAILED_SHA))
        result = subprocess.run(
            [
                "bash",
                str(SOURCE.with_name("create-getwork-actions-manifest.sh")),
                "--sha",
                FAILED_SHA,
                "--artifact-dir",
                str(stage),
                "--output",
                str(
                    self.old_proof
                    / ("henukit-getwork-actions-" + FAILED_SHA + ".manifest")
                ),
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        (stage / ("oauth-continuation-" + FAILED_SHA + ".env")).write_bytes(
            (old_runtime / "release-gates/oauth-continuation.env").read_bytes()
        )
        fixture_path = self.root / "transition-fixture.json"
        fixture = json.loads(fixture_path.read_text())
        fixture.update(candidate_sha=FAILED_SHA, run_id=41369983145)
        fixture_path.write_text(json.dumps(fixture))
        state_path = self.root / "fake-state.json"
        state = json.loads(state_path.read_text())
        for tag, image in list(state["images"].items()):
            if tag.endswith(":" + food.NEW):
                state["images"][tag.replace(food.NEW, FAILED_SHA)] = image
        state_path.write_text(json.dumps(state))
        result = subprocess.run(
            [
                sys.executable,
                str(old_runtime / "bin" / SOURCE.name),
                "--sha",
                FAILED_SHA,
                "--run-id",
                "41369983145",
                "--proof-dir",
                str(self.old_proof),
                "--test-root",
                str(self.root),
                "--acknowledge-getwork-disabled",
                "--execute",
            ],
            env=self.env,
            capture_output=True,
            text=True,
        )
        self.assertNotEqual(result.returncode, 0)
        self.old_record = (
            self.root
            / "var/lib/henukit-scoped-releases/stack-transition"
            / FAILED_SHA
            / "receipt.json"
        )
        self.assertEqual(
            json.loads(self.old_record.read_text())["status"],
            "rolling_back",
            result.stderr,
        )
        fixture.pop("candidate_sha")
        fixture.pop("run_id")
        fixture_path.write_text(json.dumps(fixture))
        (self.root / "calls.jsonl").write_text("")
        return json.loads(self.old_record.read_text())

    def recovery(self, extra=None, arguments=()):
        return self.transition(
            "--rollback",
            extra,
            (
                "--recover-transition",
                FAILED_SHA,
                "--recovery-proof-dir",
                str(self.old_proof),
                *arguments,
            ),
        )

    def test_pinned_successor_recovers_real_failed_receipt_without_changing_identity(
        self,
    ):
        original = self.failed_predecessor_fixture()
        immutable = {
            k: v
            for k, v in original.items()
            if k not in ("status", "updated_at", "rollback_checkpoint")
        }
        result = self.recovery()
        self.assertEqual(result.returncode, 0, result.stderr)
        data = json.loads(self.old_record.read_text())
        self.assertEqual(data["status"], "rolled_back")
        self.assertEqual({k: data[k] for k in immutable}, immutable)
        self.assertIn("recovery_authority", data)
        before = (self.root / "fake-state.json").read_bytes()
        result = self.recovery()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "already_rolled_back")
        self.assertEqual(before, (self.root / "fake-state.json").read_bytes())
        self.assertFalse(
            any(name == "docker" and "load" in args for name, args in self.calls())
        )

    def test_recovered_checkpoint_allows_next_transition_with_real_recreated_ids(self):
        original = self.failed_predecessor_fixture()
        before = {
            p: p.read_bytes()
            for p in (self.full_record, self.membership_record, self.portal_record)
        }
        result = self.recovery()
        self.assertEqual(result.returncode, 0, result.stderr)
        recovered = json.loads(self.old_record.read_text())
        self.assertNotEqual(recovered["rollback_checkpoint"], original["selected"])
        result = self.transition("--preflight")
        self.assertEqual(result.returncode, 0, result.stderr)
        result = self.transition("--execute")
        self.assertEqual(result.returncode, 0, result.stderr)
        new = json.loads(
            (self.old_record.parent.with_name(food.NEW) / "receipt.json").read_text()
        )
        self.assertEqual(new["selected"], recovered["rollback_checkpoint"])
        self.assertIn("recovered_transition", new)
        self.assertEqual(before, {p: p.read_bytes() for p in before})

    def test_recovered_checkpoint_rejects_foreign_recreation_and_evidence_drift(self):
        self.failed_predecessor_fixture()
        result = self.recovery()
        self.assertEqual(result.returncode, 0, result.stderr)
        state_path = self.root / "fake-state.json"
        state = state_path.read_bytes()
        record = self.old_record.read_bytes()
        data = json.loads(record)
        audit = self.old_record.parent / data["recovery_authority"][food.NEW]["file"]
        audit_bytes = audit.read_bytes()
        for kind in (
            "foreign_id",
            "foreign_environment",
            "reset_checkpoint",
            "audit",
            "not_terminal",
        ):
            with self.subTest(kind=kind):
                state_path.write_bytes(state)
                self.old_record.write_bytes(record)
                audit.chmod(0o600)
                audit.write_bytes(audit_bytes)
                audit.chmod(0o400)
                changed = json.loads(state)
                if kind == "foreign_id":
                    changed["containers"]["henukit-portal-1"]["Id"] = (
                        "foreign-recreation"
                    )
                    state_path.write_text(json.dumps(changed))
                elif kind == "foreign_environment":
                    changed["containers"]["henukit-account-portfolio-1"]["Config"][
                        "Env"
                    ].append("FOREIGN=1")
                    state_path.write_text(json.dumps(changed))
                elif kind == "reset_checkpoint":
                    self.old_record.write_text(
                        json.dumps({**data, "rollback_checkpoint": data["selected"]})
                    )
                elif kind == "audit":
                    audit.chmod(0o600)
                    audit.write_text('{"authority":{}}')
                    audit.chmod(0o400)
                else:
                    self.old_record.write_text(
                        json.dumps({**data, "status": "rolling_back"})
                    )
                before = (
                    state_path.read_bytes(),
                    self.envfile.read_bytes(),
                    self.old_record.read_bytes(),
                )
                (self.root / "calls.jsonl").write_text("")
                result = self.transition("--execute")
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(
                    before,
                    (
                        state_path.read_bytes(),
                        self.envfile.read_bytes(),
                        self.old_record.read_bytes(),
                    ),
                )
                self.assertFalse(
                    any(
                        name == "docker" and ("load" in args or "up" in args)
                        for name, args in self.calls()
                    )
                )

    def test_recovery_ignores_unpublished_audit_temporary_and_rejects_foreign_final(
        self,
    ):
        self.failed_predecessor_fixture()
        orphan = self.old_record.parent / ".recovery-incoming-interrupted"
        orphan.write_bytes(b'{"authority":')
        orphan.chmod(0o400)
        final = self.old_record.parent / ("recovery-" + food.NEW + "-1.json")
        final.write_text('{"authority":{"sha":"foreign"}}')
        final.chmod(0o400)
        before = self.old_record.read_bytes()
        result = self.recovery()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.old_record.read_bytes(), before)
        final.unlink()
        result = self.recovery()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            json.loads(self.old_record.read_text())["status"], "rolled_back"
        )
        self.assertEqual(orphan.read_bytes(), b'{"authority":')

    def test_successor_recovery_rejects_foreign_authority_before_online_mutation(self):
        original = self.failed_predecessor_fixture()
        for field, value in (
            ("sha", "d" * 40),
            ("run_id", "999"),
            ("attempt", 2),
            ("helper_sha256", "0" * 64),
            ("status", "active"),
            ("status", "accepted"),
            ("scope", ["portal"]),
        ):
            with self.subTest(field=field, value=value):
                self.old_record.write_text(json.dumps({**original, field: value}))
                before = (self.root / "fake-state.json").read_bytes()
                result = self.recovery()
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(before, (self.root / "fake-state.json").read_bytes())
                self.assertFalse(list(self.old_record.parent.glob("recovery-*.json")))
        self.old_record.write_text(json.dumps(original))
        for extra in (
            {"STALE_SOURCE": "1"},
            {"BAD_OLD_PROOF": "1"},
            {"OLD_PROOF_ATTEMPT": "2"},
            {"OLD_ATTEMPT_FAILED": "1"},
            {"NEW_PROOF_RUN": "999"},
            {"NEW_PROOF_ATTEMPT": "2"},
        ):
            with self.subTest(extra=extra):
                before = (self.root / "fake-state.json").read_bytes()
                result = self.recovery(extra)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(before, (self.root / "fake-state.json").read_bytes())
                self.assertFalse(list(self.old_record.parent.glob("recovery-*.json")))
        self.assertFalse(
            any(
                name == "docker" and ("up" in args or "load" in args)
                for name, args in self.calls()
            )
        )

    def strict_metadata_fixture(self):
        self.full_fixture()
        fixture_path = self.root / "transition-fixture.json"
        fixture = json.loads(fixture_path.read_text())
        fixture["strict_metadata"] = True
        current = json.loads(self.full_record.read_text())
        state_path = self.root / "fake-state.json"
        state = json.loads(state_path.read_text())
        for service, sha, deployed in (
            ("portal", current["sha"], current["deployed_at"]),
            ("portal-summary", food.OLD, FULL_DEPLOYED_AT),
        ):
            spec = fixture["base"]["services"][service]
            spec["environment"].update(PORTAL_VERSION=sha, PORTAL_DEPLOYED_AT=deployed)
            actual = state["containers"]["henukit-" + service + "-1"]
            actual["Config"]["Env"] = [
                key + "=" + str(value) for key, value in spec["environment"].items()
            ]
            actual["Config"]["Labels"]["com.docker.compose.config-hash"] = (
                "config-" + service + "-" + sha + "-" + deployed
            )
        fixture_path.write_text(json.dumps(fixture))
        state_path.write_text(json.dumps(state))

    def test_mixed_base_uses_full_metadata_and_candidate_timestamp_is_one_value(self):
        self.strict_metadata_fixture()
        environment = self.envfile.read_bytes()
        result = self.transition("--execute", {"DELAY_GATE_METADATA": "1"})
        self.assertEqual(result.returncode, 0, result.stderr)
        record = (
            self.root
            / "var/lib/henukit-scoped-releases/stack-transition"
            / food.NEW
            / "receipt.json"
        )
        data = json.loads(record.read_text())
        self.assertEqual(
            data["sources"]["portal-summary"]["deployed"], FULL_DEPLOYED_AT
        )
        self.assertEqual(
            data["before"]["services"]["portal-summary"]["environment"][
                "PORTAL_DEPLOYED_AT"
            ],
            FULL_DEPLOYED_AT,
        )
        for service in ("portal", "portal-summary"):
            self.assertEqual(
                data["after"]["services"][service]["environment"]["PORTAL_DEPLOYED_AT"],
                data["deployed_at"],
            )
        self.assertTrue(self.envfile.read_bytes().startswith(environment))

    def test_unknown_or_missing_full_metadata_fails_closed(self):
        self.strict_metadata_fixture()
        original = self.envfile.read_text()
        for value in (
            None,
            "2026-02-30T05:05:00Z",
            "2026-10-18T13:05:00+08:00",
            "duplicate",
        ):
            with self.subTest(value):
                line = "PORTAL_DEPLOYED_AT=" + FULL_DEPLOYED_AT
                changed = original.replace(
                    line,
                    ""
                    if value is None
                    else line + "\nPORTAL_DEPLOYED_AT=" + FULL_DEPLOYED_AT
                    if value == "duplicate"
                    else "PORTAL_DEPLOYED_AT=" + value,
                )
                self.envfile.write_text(changed)
                member = json.loads(self.membership_record.read_text())
                member["environment_sha256"] = self.digest(self.envfile)
                (self.membership_record.parent / "configuration.backup").write_bytes(
                    self.envfile.read_bytes()
                )
                self.membership_record.write_text(json.dumps(member))
                current = json.loads(self.full_record.read_text())
                current["membership_checkpoint"]["sha256"] = self.digest(
                    self.membership_record
                )
                self.full_record.write_text(json.dumps(current))
                result = self.transition("--preflight")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("full release timestamp", result.stderr)
                self.assertEqual(self.envfile.read_text(), changed)

    def test_full_metadata_rotation_during_loading_is_preserved_and_refused(self):
        self.strict_metadata_fixture()
        (self.root / "calls.jsonl").unlink()
        result = self.transition("--execute", {"DRIFT_ENV_DURING_LOAD": "timestamp"})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("environment", result.stderr)
        self.assertIn(
            "PORTAL_DEPLOYED_AT=2026-10-19T05:05:00Z", self.envfile.read_text()
        )
        self.assertFalse(
            any(name == "docker" and "up" in args for name, args in self.calls())
        )
        self.assertEqual(
            (self.root / "usr/local/bin/henukit-deploy-webhook").read_bytes(),
            b"previous materials runtime",
        )

    def test_success_commits_full_aliases_and_same_candidate_does_not_replay(self):
        self.full_fixture()
        immutable = {
            path: path.read_bytes()
            for path in (self.full_record, self.membership_record, self.portal_record)
        }
        old_environment = self.envfile.read_bytes()
        result = self.transition("--execute")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "active")
        self.assertFalse(self.marker.exists())
        for path in (
            self.root / "opt/henukit/current",
            self.root / "opt/henukit-current",
        ):
            self.assertEqual(path.resolve(), self.runtime)
        self.assertTrue(self.envfile.read_bytes().startswith(old_environment))
        self.assertIn(b"CAREER_GETWORK_ENABLED=0\n", self.envfile.read_bytes())
        self.assertEqual(immutable, {path: path.read_bytes() for path in immutable})
        self.assertEqual(
            (self.root / "usr/local/bin/henukit-deploy-webhook").read_bytes(),
            b"candidate materials runtime",
        )
        (self.root / "calls.jsonl").unlink()
        result = self.transition("--execute")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "already_active")
        self.assertFalse(
            any(
                name == "docker" and ("up" in args or "load" in args)
                for name, args in self.calls()
            )
        )

    def test_failed_smoke_restores_mixed_images_private_configuration_and_stopped_career(
        self,
    ):
        self.full_fixture()
        before = json.loads((self.root / "fake-state.json").read_text())["containers"]
        environment = self.envfile.read_bytes()
        marker = self.marker.resolve()
        immutable = {
            path: path.read_bytes()
            for path in (self.full_record, self.membership_record, self.portal_record)
        }
        result = self.transition("--execute", {"FAIL_SMOKE": "1"})
        self.assertNotEqual(result.returncode, 0)
        data = json.loads(
            (
                self.root
                / "var/lib/henukit-scoped-releases/stack-transition"
                / food.NEW
                / "receipt.json"
            ).read_text()
        )
        self.assertEqual(data["status"], "rolled_back", result.stderr)
        after = json.loads((self.root / "fake-state.json").read_text())["containers"]
        for name, old in before.items():
            self.assertEqual(
                after[name]["Config"]["Image"], old["Config"]["Image"], name
            )
            self.assertEqual(after[name]["Image"], old["Image"], name)
            self.assertEqual(
                after[name]["State"]["Running"], old["State"]["Running"], name
            )
        self.assertEqual(
            after["henukit-nginx-1"]["Mounts"], before["henukit-nginx-1"]["Mounts"]
        )
        self.assertEqual(self.envfile.read_bytes(), environment)
        self.assertEqual(self.marker.resolve(), marker)
        self.assertEqual(immutable, {path: path.read_bytes() for path in immutable})
        self.assertEqual(
            (self.root / "usr/local/bin/henukit-deploy-webhook").read_bytes(),
            b"previous materials runtime",
        )
        self.assertNotIn("ssssssssssssssss", result.stdout + result.stderr)
        for name, args in self.calls():
            if (
                name == "docker"
                and "up" in args
                and any("mixed.compose.json" in arg for arg in args)
            ):
                selected = args[args.index("--no-deps") + 1 :]
                if any(service.startswith("career-") for service in selected):
                    self.assertIn("--no-start", args)

    def test_unknown_service_or_schema_drift_fails_before_any_online_write(self):
        self.full_fixture()
        (self.root / "calls.jsonl").unlink()
        result = self.transition("--execute", {"UNKNOWN_SCOPE": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("scope", result.stderr)
        self.assertFalse(
            any(
                name == "docker" and any(x in args for x in ("up", "load", "exec"))
                for name, args in self.calls()
            )
        )
        migration = self.runtime / "migrations/platform-core/002.up.sql"
        migration.write_text("ALTER TABLE users ADD COLUMN unexpected text;")
        self.repack()
        (self.root / "calls.jsonl").unlink()
        result = self.transition("--execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("schema changes: platform-core", result.stderr)
        self.assertFalse(
            any(
                name == "docker" and any(x in args for x in ("up", "load", "exec"))
                for name, args in self.calls()
            )
        )

    def test_main_or_run_attempt_drift_during_image_loading_prevents_online_change(
        self,
    ):
        self.assert_load_drift("main")

    def test_new_run_attempt_during_image_loading_prevents_online_change(self):
        self.assert_load_drift("attempt")

    def assert_load_drift(self, kind):
        self.full_fixture()
        (self.root / "calls.jsonl").unlink()
        result = self.transition("--execute", {"DRIFT_DURING_LOAD": kind})
        self.assertNotEqual(result.returncode, 0)
        data = json.loads(
            (
                self.root
                / "var/lib/henukit-scoped-releases/stack-transition"
                / food.NEW
                / "receipt.json"
            ).read_text()
        )
        self.assertEqual(data["status"], "rolled_back", result.stderr)
        self.assertEqual(
            (self.root / "usr/local/bin/henukit-deploy-webhook").read_bytes(),
            b"previous materials runtime",
        )
        self.assertFalse(
            any(name == "docker" and "up" in args for name, args in self.calls())
        )

    def test_interrupted_material_install_recovers_with_orphan_receipt_temporary(self):
        self.full_fixture()
        environment = self.envfile.read_bytes()
        result = self.transition("--execute", {"INTERRUPT_INSTALL": "1"})
        self.assertEqual(result.returncode, -9, result.stderr)
        folder = (
            self.root / "var/lib/henukit-scoped-releases/stack-transition" / food.NEW
        )
        data = json.loads((folder / "receipt.json").read_text())
        self.assertEqual(data["status"], "materials_installing")
        (folder / "receipt.incoming").write_text("orphan partial write")
        (folder / "receipt.incoming").chmod(0o600)
        result = self.transition("--execute")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("interrupted transition restored", result.stderr)
        self.assertEqual(
            json.loads((folder / "receipt.json").read_text())["status"], "rolled_back"
        )
        self.assertEqual(self.envfile.read_bytes(), environment)
        self.assertEqual(
            (self.root / "usr/local/bin/henukit-deploy-webhook").read_bytes(),
            b"previous materials runtime",
        )
        (self.root / "calls.jsonl").unlink()
        result = self.transition("--rollback")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "already_rolled_back")
        self.assertFalse(
            any(name == "docker" and "up" in args for name, args in self.calls())
        )

    def test_pending_materials_work_can_start_after_an_accepted_commit(self):
        self.full_fixture()
        path = self.root / "fake-state.json"
        state = json.loads(path.read_text())
        state["materials_path"] = "enabled"
        path.write_text(json.dumps(state))
        result = self.transition("--execute", {"START_RUNNER_ON_ENABLE": "1"})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "active")
        state = json.loads(path.read_text())
        self.assertTrue(state["runner_started"])
        self.assertEqual(state["materials_path"], "enabled")
        self.assertTrue(state["deployed"])
        self.assertEqual((self.root / "opt/henukit/current").resolve(), self.runtime)

    def test_candidate_owner_database_route_change_is_refused_before_backups(self):
        self.full_fixture()
        (self.root / "calls.jsonl").unlink()
        result = self.transition("--execute", {"BAD_DATA_ROUTE": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("owner data-route changes", result.stderr)
        self.assertFalse(
            any(
                name == "docker" and any(x in args for x in ("up", "load", "exec"))
                for name, args in self.calls()
            )
        )

    def test_recovery_refuses_foreign_selected_configuration_without_overwriting_it(
        self,
    ):
        self.full_fixture()
        result = self.transition("--execute", {"INTERRUPT_INSTALL": "1"})
        self.assertEqual(result.returncode, -9, result.stderr)
        path = self.root / "fake-state.json"
        state = json.loads(path.read_text())
        state["containers"]["henukit-portal-1"]["Config"]["Env"] = ["SECRET=foreign"]
        path.write_text(json.dumps(state))
        before = path.read_bytes()
        (self.root / "calls.jsonl").unlink()
        result = self.transition("--rollback")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("selected container drifted", result.stderr)
        self.assertEqual(path.read_bytes(), before)
        self.assertFalse(
            any(name == "docker" and "up" in args for name, args in self.calls())
        )

    def test_existing_production_gate_rejects_mock_or_disabled_payments(self):
        self.full_fixture()
        original = self.envfile.read_text()
        for old, new in (
            ("PORTAL_API_MODE=live", "PORTAL_API_MODE=mock"),
            (
                "ACCOUNT_PORTFOLIO_EASYPAY_ENABLED=1",
                "ACCOUNT_PORTFOLIO_EASYPAY_ENABLED=0",
            ),
        ):
            with self.subTest(new):
                self.envfile.write_text(original.replace(old, new))
                member = json.loads(self.membership_record.read_text())
                member["environment_sha256"] = self.digest(self.envfile)
                (self.membership_record.parent / "configuration.backup").write_bytes(
                    self.envfile.read_bytes()
                )
                self.membership_record.write_text(json.dumps(member))
                current = json.loads(self.full_record.read_text())
                current["membership_checkpoint"]["sha256"] = self.digest(
                    self.membership_record
                )
                self.full_record.write_text(json.dumps(current))
                (self.root / "calls.jsonl").unlink()
                result = self.transition("--preflight")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("production data boundary gate failed", result.stderr)
                self.assertFalse(
                    any(
                        name == "docker" and ("up" in args or "load" in args)
                        for name, args in self.calls()
                    )
                )

    def test_environment_change_during_image_loading_is_preserved_and_blocks_cutover(
        self,
    ):
        self.full_fixture()
        (self.root / "calls.jsonl").unlink()
        result = self.transition("--execute", {"DRIFT_ENV_DURING_LOAD": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("environment", result.stderr)
        self.assertIn(b"FOREIGN_CHANGE=1\n", self.envfile.read_bytes())
        self.assertEqual(
            (self.root / "usr/local/bin/henukit-deploy-webhook").read_bytes(),
            b"previous materials runtime",
        )
        calls = [
            (name, args)
            for name, args in self.calls()
            if name == "docker" and "up" in args
        ]
        self.assertFalse(
            any(any(".compose.json" in arg for arg in args) for _, args in calls)
        )


def load_tests(loader, tests, pattern):
    suite = loader.loadTestsFromTestCase(StackTransitionCLI)
    for name in (
        "test_transition_requires_actual_active_v3_without_online_mutation",
        "test_complete_preflight_binds_real_mixed_checkpoint_without_mutation",
        "test_workshop_boundaries_use_console_and_private_owner_before_mutation",
        "test_pinned_successor_recovers_real_failed_receipt_without_changing_identity",
        "test_recovered_checkpoint_allows_next_transition_with_real_recreated_ids",
        "test_successor_recovery_rejects_foreign_authority_before_online_mutation",
        "test_recovered_checkpoint_rejects_foreign_recreation_and_evidence_drift",
        "test_recovery_ignores_unpublished_audit_temporary_and_rejects_foreign_final",
        "test_success_commits_full_aliases_and_same_candidate_does_not_replay",
        "test_failed_smoke_restores_mixed_images_private_configuration_and_stopped_career",
        "test_unknown_service_or_schema_drift_fails_before_any_online_write",
        "test_main_or_run_attempt_drift_during_image_loading_prevents_online_change",
        "test_new_run_attempt_during_image_loading_prevents_online_change",
        "test_interrupted_material_install_recovers_with_orphan_receipt_temporary",
        "test_pending_materials_work_can_start_after_an_accepted_commit",
        "test_candidate_owner_database_route_change_is_refused_before_backups",
        "test_recovery_refuses_foreign_selected_configuration_without_overwriting_it",
        "test_existing_production_gate_rejects_mock_or_disabled_payments",
        "test_environment_change_during_image_loading_is_preserved_and_blocks_cutover",
        "test_explicit_materials_baseline_preserves_independent_delivered_capabilities",
        "test_independent_materials_rejects_missing_unverified_or_failed_attempt_proof",
        "test_independent_materials_rejects_partial_live_target_drift",
        "test_independent_materials_retained_bytes_drift_after_preflight_is_refused",
        "test_failed_transition_restores_the_independent_materials_cohort",
        "test_interrupted_independent_materials_refuses_foreign_identity_then_restores_B",
        "test_mixed_base_uses_full_metadata_and_candidate_timestamp_is_one_value",
        "test_unknown_or_missing_full_metadata_fails_closed",
        "test_full_metadata_rotation_during_loading_is_preserved_and_refused",
    ):
        suite.addTest(StackTransitionCheckpoint(name))
    return suite


if __name__ == "__main__":
    unittest.main()
