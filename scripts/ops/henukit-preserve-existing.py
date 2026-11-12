#!/usr/bin/env python3
"""Read-only authority verifier for the existing full release owner.

Only capture writes: an immutable digest/identity receipt, never credential bytes.
Docker config/inspect and a read-only PostgreSQL transaction are the only probes.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys

PERMISSIONS = {'account.tickets.read', 'account.tickets.reply', 'account.tickets.transition',
               'account.membership.write', 'account.points.adjust', 'account.orders.read',
               'account.orders.close', 'account.orders.refund'}
VOLATILE = {'PORTAL_VERSION', 'PORTAL_COMMIT_SHA', 'PORTAL_DEPLOYED_AT', 'RELEASE_SHA'}


def require(value):
    if not value:
        raise ValueError('existing authority missing or changed')


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


def trusted(path, private=False, directory=False):
    anchor = Path(os.environ.get('HENUKIT_TRUST_ANCHOR', '/'))
    require(path.is_absolute() and '..' not in path.parts and path.is_relative_to(anchor))
    for item in (path, *path.parents):
        info = item.lstat()
        require(info.st_uid == os.geteuid() and not info.st_mode & 0o022 and not stat.S_ISLNK(info.st_mode))
        require(stat.S_ISREG(info.st_mode) if item == path and not directory else stat.S_ISDIR(info.st_mode))
        if item == anchor:
            break
    if private:
        require(stat.S_IMODE(path.lstat().st_mode) in (0o400, 0o600))


def file_identity(path):
    source_config = Path(os.environ.get('HENUKIT_SOURCE_DEPLOY_CONFIG', '/etc/henukit-deploy/deploy.env'))
    trusted(path, path != source_config)
    if path == source_config:
        require(stat.S_IMODE(path.stat().st_mode) in (0o400, 0o600, 0o640))
    with path.open('rb') as stream:
        info = os.fstat(stream.fileno())
        result = {key: getattr(info, 'st_' + key) for key in ('dev', 'ino', 'uid', 'gid', 'mode', 'size', 'mtime_ns', 'ctime_ns')}
        result['sha256'] = hashlib.sha256(stream.read()).hexdigest()
    after = path.stat()
    require(all(getattr(info, 'st_' + key) == getattr(after, 'st_' + key) for key in ('dev', 'ino', 'uid', 'gid', 'mode', 'size', 'mtime_ns', 'ctime_ns')))
    return result


def docker(*args, input=None, env=None):
    result = subprocess.run(['docker', *args], input=input, env=env, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60)
    require(result.returncode == 0)
    return result.stdout


def configuration(args, runtime):
    compose_file = args.release_dir / 'docker-compose.henukit.release.yml'
    trusted(compose_file)
    # Scrub ambient Compose overrides. Both validation and actual activation must
    # use the original file, not a newly rendered secret-bearing env file.
    environment = os.environ.copy()
    for line in args.env_file.read_text().splitlines():
        match = re.match(r'\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=', line)
        if match:
            environment.pop(match[1], None)
    environment.update(RELEASE_SHA=args.sha, PORTAL_VERSION=args.sha,
                       PORTAL_DEPLOYED_AT='1970-01-01T00:00:00Z')
    compose = ['compose', '--env-file', str(args.env_file), '-f', str(compose_file)]
    config = json.loads(docker(*compose, 'config', '--format', 'json', env=environment))
    actual_environment = {**os.environ, 'RELEASE_SHA': args.sha, 'PORTAL_VERSION': args.sha,
                          'PORTAL_DEPLOYED_AT': '1970-01-01T00:00:00Z'}
    require(config == json.loads(docker(*compose, 'config', '--format', 'json', env=actual_environment)))
    require(not config.get('secrets') and not config.get('configs'))
    result = {}
    public_mounts = {}
    for service, value in config['services'].items():
        expected = value.get('environment', {})
        require(isinstance(expected, dict) and all(v is not None for v in expected.values()))
        # Bind mounts can introduce an untracked credential source. This mode
        # supports the existing env-only application contract; fail on additions.
        for mount in value.get('volumes', []):
            if mount.get('type') == 'bind':
                source = Path(mount['source'])
                require(mount.get('read_only') is True)
                if source == Path('/opt/henukit-materials/public') and mount.get('target') == '/srv/materials':
                    trusted(source, directory=True)
                    info = source.lstat()
                    public_mounts[service] = {k: getattr(info, 'st_' + k) for k in ('dev','ino','uid','gid','mode')}
                else:
                    require(source.is_relative_to(args.release_dir / 'infra'))
                    trusted(source)
        stable = {k: str(v) for k, v in expected.items() if k not in VOLATILE}
        result[service] = stable
        if runtime:
            container = docker(*compose, 'ps', '-q', service, env=environment).strip().splitlines()
            require(len(container) == 1)
            inspected = json.loads(docker('inspect', container[0]))
            live = dict(item.split('=', 1) for item in inspected[0]['Config']['Env'])
            require(all(live.get(k) == v for k, v in stable.items()))
    require(result.get('portal-gateway', {}).get('QUIZCRAFT_CORE_URL') == 'http://quizcraft:10089')
    account = result.get('account-portfolio', {})
    require(account.get('ACCOUNT_PORTFOLIO_EASYPAY_ENABLED') == '1')
    require(re.fullmatch(r'[A-Za-z0-9_-]{1,64}', account.get('ACCOUNT_PORTFOLIO_EASYPAY_PID', '')))
    key = account.get('ACCOUNT_PORTFOLIO_EASYPAY_KEY', '')
    require(len(key) >= 16 and not re.search(r'\s|replace|example|changeme|test-secret', key, re.I))
    require(account.get('ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL') == 'https://metaview.top/epay')
    require(account.get('ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL') == 'https://henukit.cn/api/v1/payment-providers/easypay/notifications')
    require(account.get('ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL') == 'https://henukit.cn/account/membership')
    args.tenant_fingerprint = hashlib.sha256((account['ACCOUNT_PORTFOLIO_EASYPAY_PID'] + '\n' + key).encode()).hexdigest()
    return digest({'environments': result, 'public_mounts': public_mounts})


def grants(args):
    require(re.fullmatch(r'[a-z][a-z0-9-]{1,63}', args.role))
    # Literal role is constrained above. One stable read-only snapshot includes
    # bindings, role revision and prior grant audits; no owner grant is invoked.
    sql = f"""BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT json_build_object(
 'role', (SELECT row_to_json(r) FROM authorization_roles r WHERE code='{args.role}'),
 'permissions', (SELECT json_agg(p.code ORDER BY p.code) FROM permission_codes p
   JOIN role_permissions rp ON rp.permission_code=p.code
   JOIN authorization_roles r ON r.id=rp.role_id WHERE r.code='{args.role}' AND p.status='active'),
 'bindings', (SELECT coalesce(json_agg(g ORDER BY g.id), '[]') FROM user_role_grants g
   JOIN authorization_roles r ON r.id=g.role_id WHERE r.code='{args.role}'),
 'audit', (SELECT coalesce(json_agg(a ORDER BY a.id), '[]') FROM account_operator_role_grant_audit_events a WHERE a.role_code='{args.role}'));
ROLLBACK;"""
    value = json.loads(docker('exec', '-i', os.environ.get('HENUKIT_POSTGRES_CONTAINER', 'henukit-postgres-1'),
                              'sh', '-ceu', 'psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d platform', input=sql))
    require(value['role']['code'] == args.role and value['role']['status'] == 'active')
    require(PERMISSIONS <= set(value['permissions']))
    return digest(value)


def gateway(args):
    patches = args.release_dir / 'infra/epay-gateway/patches'
    manifest = ''
    for name in ('0001-henukit-query-and-notify-outbox.patch',
                 '0002-henukit-close-refund-and-response-verification.patch',
                 '0003-henukit-private-checkout-handle.patch'):
        path = patches / name
        trusted(path)
        manifest += hashlib.sha256(path.read_bytes()).hexdigest() + '  ' + name + '\n'
    target = os.environ.get('HENUKIT_EPAY_GATEWAY_SSH_TARGET', 'root@metaview.top')
    directory = os.environ.get('HENUKIT_EPAY_GATEWAY_DIR', '/root/epay-gateway')
    require(re.fullmatch(r'[A-Za-z0-9_.@-]+', target) and re.fullmatch(r'/[A-Za-z0-9_./-]+', directory)
            and '..' not in Path(directory).parts)
    # Never return tenant credentials. The established SSH channel validates the
    # remote in place and returns only the protected files' identity/digest.
    script = r"""
import hashlib,json,os,pathlib,re,stat,sys,urllib.request
root,expected,pair=sys.argv[1:];root=pathlib.Path(root)
def identity(path):
 info=path.lstat()
 assert stat.S_ISREG(info.st_mode) and info.st_uid==0 and not info.st_mode&0o077
 for parent in path.parents:
  item=parent.lstat();assert stat.S_ISDIR(item.st_mode) and item.st_uid==0 and not item.st_mode&0o022
 return {**{k:getattr(info,'st_'+k) for k in ('dev','ino','uid','gid','mode')},'sha256':hashlib.sha256(path.read_bytes()).hexdigest()}
env=root/'.env';marker=root/'.henukit-patches.sha256'
value={'env':identity(env),'patches':identity(marker)}
assert value['patches']['sha256']==expected
values={}
for line in env.read_text().splitlines():
 match=re.match(r'\s*(HENUKIT_EPAY_[A-Z_]+)\s*=(.*)',line)
 if match:
  assert match[1] not in values;values[match[1]]=match[2].strip()
assert values['HENUKIT_EPAY_ENABLED']=='1'
assert hashlib.sha256((values['HENUKIT_EPAY_PID']+'\n'+values['HENUKIT_EPAY_KEY']).encode()).hexdigest()==pair
assert 'https://henukit.cn/api/v1/payment-providers/easypay/notifications' in values['HENUKIT_EPAY_NOTIFY_URLS'].split(',')
assert 'https://henukit.cn' in values['HENUKIT_EPAY_RETURN_ORIGINS'].split(',')
with urllib.request.urlopen('http://127.0.0.1:9219/health',timeout=5) as response:assert response.status==200
print(json.dumps(value,sort_keys=True))
"""
    result = subprocess.run(['ssh', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes', target,
                             'python3', '-I', '-', directory, hashlib.sha256(manifest.encode()).hexdigest(),
                             args.tenant_fingerprint], input=script, text=True, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, timeout=30)
    require(result.returncode == 0)
    value = json.loads(result.stdout)
    require(set(value) == {'env', 'patches'})
    return digest(value)


def authority_files(args):
    files = [args.env_file]
    # Existing production authority only. The isolated fixtures use their own
    # root, just like the original watcher's trust-anchor contract.
    if Path(os.environ.get('HENUKIT_TRUST_ANCHOR', '/')) == Path('/'):
        materials = Path('/etc/henukit-deploy/materials-activate.env')
        files += [Path(os.environ.get('GH_TOKEN_FILE', '/etc/henukit/github-actions-read.token')),
                  Path(os.environ.get('HENUKIT_SOURCE_DEPLOY_CONFIG', '/etc/henukit-deploy/deploy.env')),
                  Path('/etc/henukit-deploy/webhook-secret'), materials]
        trusted(materials, True)
        require(not re.search(r'^HENUKIT_MATERIALS_(CONVERTER|IMPORTER|PSQL|PG_SERVICE_FILE|PG_SERVICE|LEGACY_INVENTORY)=',
                              materials.read_text(), re.M))
        for path in ('/etc/henukit-deploy/materials-postgresql.conf', '/etc/henukit-deploy/materials-legacy-inventory.json'):
            require(not os.path.lexists(path))
    return {str(path): file_identity(path) for path in files}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=('capture', 'check', 'candidate', 'runtime'))
    parser.add_argument('--sha', required=True)
    parser.add_argument('--env-file', type=Path, required=True)
    parser.add_argument('--receipt', type=Path, required=True)
    parser.add_argument('--release-dir', type=Path, required=True)
    parser.add_argument('--role', required=True)
    args = parser.parse_args()
    require(re.fullmatch(r'[a-f0-9]{40}', args.sha))
    saved = None
    if args.receipt.exists() or args.receipt.is_symlink():
        trusted(args.receipt, True)
        require(stat.S_IMODE(args.receipt.stat().st_mode) == 0o400)
        saved = json.loads(args.receipt.read_bytes())
    baseline_dir = saved['baseline_dir'] if saved else str(args.release_dir)
    if args.mode == 'check':
        args.release_dir = Path(baseline_dir)
    identity = authority_files(args)
    snapshot = {'format': 'henukit-preserve-existing-v1', 'sha': args.sha,
                'env_file': str(args.env_file), 'files': identity, 'baseline_dir': baseline_dir,
                'verifier': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                'grants': grants(args)}
    snapshot['configuration'] = configuration(args, args.mode in ('capture', 'runtime'))
    snapshot['gateway'] = gateway(args)
    require(authority_files(args) == identity)
    if args.receipt.exists() or args.receipt.is_symlink():
        trusted(args.receipt, True)
        require(stat.S_IMODE(args.receipt.stat().st_mode) == 0o400)
        saved = json.loads(args.receipt.read_bytes())
        require(set(saved) == set(snapshot))
        require(all(saved.get(k) == v for k, v in snapshot.items()))
    else:
        require(args.mode == 'capture')
        # The owner validates its private state directory before invoking us.
        parent = args.receipt.parent
        require(parent.is_dir() and not parent.is_symlink() and parent.stat().st_uid == os.geteuid()
                and not parent.stat().st_mode & 0o077)
        fd = os.open(args.receipt, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o400)
        with os.fdopen(fd, 'w') as stream:
            json.dump(snapshot, stream, sort_keys=True)
            stream.write('\n')
            stream.flush()
            os.fsync(stream.fileno())
        directory = os.open(parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, KeyError, TypeError, AssertionError, subprocess.SubprocessError):
        sys.exit('preservation check failed: existing configuration, file identity or grants unavailable/changed; operator review required')
