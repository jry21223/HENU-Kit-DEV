#!/usr/bin/env python3
"""One Notice management URL transaction inside the original release owner.

This is not a publisher. The watcher supplies its four inherited locks and
validated source/alias contract. No credentials or business requests are read.
The only retained environment content is the original, explicitly empty URL
line. Hashes bind the remaining bytes without copying their contents to state.
"""
import argparse
import base64
from datetime import datetime
import fcntl
import hashlib
import json
import os
from pathlib import Path
import pwd
import re
import secrets
import stat
import subprocess
import sys

FORMAT = "henukit-notice-url-transaction-v1"
URL = "http://notice:8094"
KEY = b"NOTICE_API_URL"

def require(value, message):
    if not value:
        raise RuntimeError(message)

def sha(data):
    return hashlib.sha256(data).hexdigest()

def metadata(info):
    return {"dev":info.st_dev, "ino":info.st_ino, "uid":info.st_uid,
            "gid":info.st_gid, "mode":stat.S_IMODE(info.st_mode),
            "size":info.st_size, "mtime_ns":info.st_mtime_ns}

def sync(directory):
    fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)

def trusted(path, *, directory=False, private=False):
    info = path.lstat()
    require(info.st_uid == os.geteuid() and not info.st_mode & 0o022 and
            (stat.S_ISDIR(info.st_mode) if directory else stat.S_ISREG(info.st_mode)),
            "untrusted Notice URL authority path")
    if private:
        require(not info.st_mode & 0o077, "Notice URL authority must remain private")
    return info

def read(path, *, private=False):
    expected = metadata(trusted(path, private=private))
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, "rb") as stream:
        require(metadata(os.fstat(stream.fileno())) == expected, "authority inode changed before read")
        data = stream.read()
        require(metadata(os.fstat(stream.fileno())) == expected and
                metadata(path.lstat()) == expected, "authority changed while reading")
    return data, expected

def url_line(data):
    lines = data.splitlines(keepends=True)
    matches = [(i, line) for i,line in enumerate(lines)
               if re.match(rb"^[ \t]*(?:export[ \t]+)?NOTICE_API_URL[ \t]*=", line)]
    require(len(matches) == 1, "NOTICE_API_URL must have exactly one explicit assignment")
    index, line = matches[0]
    value = line.split(b"=", 1)[1].strip()
    if value[:1] in (b"'", b'"') and value[-1:] == value[:1]:
        value = value[1:-1]
    require(value in (b"", URL.encode()), "NOTICE_API_URL has an unsupported value")
    return lines, index, line, value

def original_source_lock(path, *, production):
    # Match watcher source_proof exactly. The original production source lock
    # belongs to henukit-deploy, unlike the root-owned release state/receipts.
    if production:
        account = pwd.getpwnam("henukit-deploy")
        owner,group = account.pw_uid,account.pw_gid
        parent = path.parent.lstat()
        require(path == Path("/run/lock/henukit-deploy.lock") and stat.S_ISDIR(parent.st_mode) and
                parent.st_uid == 0 and stat.S_IMODE(parent.st_mode) == 0o1777,
                "original source lock parent changed")
    else:
        trusted(path.parent,directory=True)
        owner,group = os.geteuid(),os.getegid()
    info = path.lstat()
    require(stat.S_ISREG(info.st_mode) and info.st_uid == owner and info.st_gid == group and
            stat.S_IMODE(info.st_mode) == 0o640 and info.st_size == 0,
            "original source protocol lock differs from its protected authority profile")
    return info

class Transaction:
    def __init__(self, args):
        self.args = args
        self.env, self.state, self.releases, self.inner, self.outer, self.source = map(
            Path, (args.env_file,args.state_root,args.release_root,args.inner,args.outer,args.source_lock))
        paths = (self.env,self.state,self.releases,self.inner,self.outer,self.source,Path(args.plan))
        require(all(p.is_absolute() and ".." not in p.parts for p in paths), "authority paths must be absolute")
        production = (Path("/opt/henukit/.env.henukit"),Path("/var/lib/henukit-actions-watch"),
                      Path("/opt/henukit-releases"),Path("/opt/henukit/current"),
                      Path("/opt/henukit-current"),Path("/run/lock/henukit-deploy.lock"))
        if any(p == expected for p,expected in zip(paths,production)):
            require(paths[:6] == production and os.geteuid() == 0,
                    "production Notice URL authority cannot mix isolated overrides")
            anchor = Path("/")
        else:
            anchor = Path(os.path.commonpath([str(p.parent) for p in paths]))
            require(anchor != Path("/") and stat.S_IMODE(trusted(anchor,directory=True).st_mode) == 0o700,
                    "isolated Notice URL roots must share a private root")
        self.production = anchor == Path("/")
        for p in paths:
            for parent in p.parents:
                if parent == anchor:
                    break
                require(parent.is_relative_to(anchor), "Notice URL authority escaped its root")
                if parent == Path("/run/lock") and self.source == production[5] and p == self.source:
                    info = parent.lstat()
                    require(stat.S_ISDIR(info.st_mode) and info.st_uid == 0 and
                            stat.S_IMODE(info.st_mode) == 0o1777,"original source lock parent changed")
                    continue
                trusted(parent,directory=True)
        trusted(self.state,directory=True,private=True)
        trusted(self.releases,directory=True)
        self.lease_identity = None
        self.lock_proof()
        for name in ("rollback-contracts/pending","approvals"):
            directory = self.state/name
            if directory.exists() or directory.is_symlink():
                trusted(directory,directory=True,private=True)
                for path in directory.iterdir():
                    require(not path.is_symlink(), "pending full-release authority must reconcile first")
                    require(path.is_dir() or (args.mode == "stage" and name == "approvals" and path.name == args.sha),
                            "pending full release must reconcile before Notice URL action")
        plan_bytes,_ = read(Path(args.plan),private=True)
        self.plan = json.loads(plan_bytes); self.plan_hash = sha(plan_bytes)
        require(set(self.plan) == {"format","release_sha","environment_path","environment_metadata",
                                  "environment_sha256","expected_value","value"} and
                self.plan["format"] == "henukit-notice-url-plan-v1" and
                self.plan["release_sha"] == args.sha and self.plan["environment_path"] == str(self.env) and
                self.plan["expected_value"] == "" and self.plan["value"] == URL,
                "plan must bind this exact SHA, environment and single empty-to-canonical Notice URL")
        require(re.fullmatch(r"[0-9a-f]{64}",self.plan["environment_sha256"]) and
                re.fullmatch(r"[0-9a-f]{40}",args.sha), "invalid Notice URL plan digest or release SHA")
        marker,_ = read(self.state/"last-activated-sha",private=True)
        self.active = marker.decode().strip()
        require(re.fullmatch(r"[0-9a-f]{40}",self.active), "invalid active release marker")
        require(self.inner.is_symlink() and self.outer.is_symlink() and
                self.inner.lstat().st_uid == os.geteuid() and self.outer.lstat().st_uid == os.geteuid() and
                os.readlink(self.inner) == str(self.releases/self.active) and
                os.readlink(self.outer) == str(self.inner), "Notice URL requires the existing accepted alias pair")
        self.runtime = self.releases / args.sha
        trusted(self.runtime,directory=True)
        release,_ = read(self.runtime/"RELEASE_SHA")
        require(release.decode().strip() == args.sha, "runtime SHA differs from Notice URL target")
        self.compose = self.runtime/"docker-compose.henukit.release.yml"
        compose,_ = read(self.compose); self.compose_hash = sha(compose)
        self.receipt_dir = self.state/"notice-url"/args.sha
        self.receipt_path = self.receipt_dir/"receipt.json"

    def lock_proof(self):
        current = {}
        for fd,path in ((8,self.state/"direct-deploy.lock"),(9,self.state/"watcher.lock"),
                        (6,self.state),(7,self.source)):
            entry = original_source_lock(path,production=self.production) if fd == 7 else trusted(path,directory=(fd==6))
            held = os.fstat(fd)
            require(metadata(held) == metadata(entry),
                    "Notice URL operation must inherit the original four locks")
            identity = metadata(entry)
            if fd == 6:
                # Our own receipt directory legitimately changes this directory's
                # size/mtime; its inode/owner/group/mode must stay unchanged.
                identity = {key:identity[key] for key in ("dev","ino","uid","gid","mode")}
            current[fd] = identity
            fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
        if self.lease_identity is None:
            self.lease_identity = current
        else:
            require(current == self.lease_identity, "original four-lock lease changed; no further writes or replay allowed")

    def command(self, args):
        # Capture output: Compose may interpolate private existing configuration;
        # it must never be echoed into receipts, errors, or deployment logs.
        self.lock_proof()
        try:
            result = subprocess.run(args,capture_output=True,text=True,timeout=60)
        except subprocess.TimeoutExpired:
            raise RuntimeError("Gateway command timed out; outcome unknown, reconcile Notice URL receipt before another action") from None
        self.lock_proof()
        require(result.returncode == 0, "Gateway command failed; reconcile Notice URL receipt before another action")
        return result.stdout.strip()

    def compose_command(self):
        return ["docker","compose","--env-file",str(self.env),"-f",str(self.compose)]

    def gateway(self):
        container = self.command([*self.compose_command(),"ps","--quiet","console-gateway"])
        require(re.fullmatch(r"[A-Za-z0-9_-]+",container), "Gateway identity is absent or ambiguous")
        value = self.command(["docker","inspect","--format",
            '{{.Id}}|{{.Config.Image}}|{{.Image}}|{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}|{{index .Config.Labels "com.docker.compose.service"}}',container])
        fields = value.split("|")
        require(len(fields)==6 and fields[0]==container and fields[5]=="console-gateway",
                "Gateway runtime identity is invalid")
        gateway = dict(zip(("id","image","image_id","status","health","service"),fields))
        self.image_proof(gateway)
        return gateway

    def image_proof(self, gateway):
        require(re.fullmatch(r"henukit-console-gateway:[0-9a-f]{40}",gateway["image"]),
                "Gateway must use its exact fixed-SHA image tag")
        image_id = self.command(["docker","image","inspect","--format","{{.Id}}",gateway["image"]])
        require(image_id == gateway["image_id"],
                "Gateway fixed-SHA tag no longer resolves to its bound actual image; no config write or recreation allowed")

    def healthy(self, gateway, release_sha=None):
        return gateway["image"] == "henukit-console-gateway:"+(release_sha or self.args.sha) and \
            gateway["status"] == "running" and gateway["health"] == "healthy"

    def save(self, receipt):
        self.lock_proof()
        for directory in (self.state/"notice-url",self.receipt_dir):
            if not directory.exists():
                directory.mkdir(mode=0o700); sync(directory.parent)
            trusted(directory,directory=True,private=True)
        path = self.receipt_dir/(".receipt-"+secrets.token_hex(12))
        fd = os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
        with os.fdopen(fd,"wb") as stream:
            stream.write(json.dumps(receipt,sort_keys=True).encode()+b"\n")
            stream.flush(); os.fsync(stream.fileno())
        if self.receipt_path.exists():
            read(self.receipt_path,private=True)
        self.lock_proof()
        os.replace(path,self.receipt_path); sync(self.receipt_dir)

    def replacement(self, data, original_meta):
        self.lock_proof()
        path = self.env.parent/(".henukit-notice-url-"+secrets.token_hex(12))
        fd = os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
        with os.fdopen(fd,"wb") as stream:
            stream.write(data); stream.flush()
            os.fchown(stream.fileno(),original_meta["uid"],original_meta["gid"])
            os.fchmod(stream.fileno(),original_meta["mode"]); os.fsync(stream.fileno())
        return path,metadata(path.lstat())

    def cleanup_prepared(self, receipt):
        path = Path(receipt["prepared_file"])
        require(path.parent == self.env.parent and re.fullmatch(r"\.henukit-notice-url-[0-9a-f]{24}",path.name),
                "Notice URL prepared file escaped its environment directory")
        if path.exists() or path.is_symlink():
            data,meta = read(path,private=True)
            direction = "restored" if receipt["phase"] == "restore-prepared" else "after"
            require(sha(data) == receipt[direction+"_sha256"] and meta == receipt[direction+"_metadata"],
                    "Notice URL prepared file identity changed")
            self.lock_proof()
            path.unlink(); sync(path.parent)

    def load(self):
        data,_ = read(self.receipt_path,private=True)
        receipt = json.loads(data)
        require(receipt["format"] == FORMAT and receipt["plan_sha256"] == self.plan_hash and
                receipt["release_sha"] == self.args.sha and receipt["environment_path"] == str(self.env) and
                receipt["compose_sha256"] == self.compose_hash,
                "Notice URL receipt no longer matches its exact reviewed authority")
        require(self.active == self.args.sha or (self.args.mode == "reconcile" and
                receipt["activation_mode"] == "full" and self.active == receipt["before_active_sha"]),
                "Notice URL recovery requires its exact accepted runtime SHA")
        return receipt

    def exact_env(self, receipt, direction):
        self.lock_proof()
        data,meta = read(self.env,private=True)
        require(sha(data) == receipt[direction+"_sha256"] and meta == receipt[direction+"_metadata"],
                "Notice URL environment drift requires operator reconciliation; no bytes changed")
        self.lock_proof()
        return data,meta

    def recreate(self, gateway):
        self.command([*self.compose_command(),"config","--quiet"])
        self.image_proof(gateway)
        self.command([*self.compose_command(),"up","-d","--no-deps","--force-recreate","--no-build","--pull","never","console-gateway"])

    def reconcile(self):
        receipt = self.load()
        phase = receipt["phase"]
        if receipt["activation_mode"] == "full" and self.active == receipt["before_active_sha"] and \
                phase in ("prepared","env-staged","full-release-restored"):
            data,meta = read(self.env,private=True)
            require(sha(data) == self.plan["environment_sha256"] and url_line(data)[3] == b"" and
                    all(meta[k] == self.plan["environment_metadata"][k] for k in ("uid","gid","mode")),
                    "original full-release rollback has not restored its exact environment; no retry performed")
            gateway = self.gateway()
            require(self.healthy(gateway,receipt["before_active_sha"]) and
                    gateway["image_id"] == receipt["before_gateway"]["image_id"],
                    "original full-release runtime rollback is not proven")
            sync(self.env.parent)
            receipt.update({"phase":"full-release-restored","business_accepted":False})
            self.save(receipt)
            print(json.dumps({"phase":receipt["phase"],"release_sha":self.args.sha,"business_accepted":False})); return
        if phase == "env-staged" and self.active == self.args.sha:
            self.health(); return
        if phase == "restore-prepared":
            data,meta = read(self.env,private=True)
            require(self.gateway() == receipt["restore_before_gateway"], "restore preparation Gateway changed")
            if sha(data) == receipt["after_sha256"] and meta == receipt["after_metadata"]:
                self.cleanup_prepared(receipt)
                receipt["phase"] = receipt["restore_origin_phase"]
            else:
                self.exact_env(receipt,"restored")
                sync(self.env.parent)
                receipt["phase"] = "restore-environment-applied"
            self.save(receipt)
        elif phase == "prepared" and receipt["activation_mode"] == "configuration":
            data,meta = read(self.env,private=True)
            gateway = self.gateway()
            require(gateway == receipt["before_gateway"], "prepared operation Gateway changed unexpectedly")
            if sha(data) == receipt["before_sha256"] and meta == receipt["before_metadata"]:
                self.cleanup_prepared(receipt)
                receipt["phase"] = "not-applied"
            else:
                self.exact_env(receipt,"after")
                sync(self.env.parent)
                receipt.update({"phase":"environment-applied","after_gateway":gateway})
            self.save(receipt)
        elif phase == "not-applied":
            self.exact_env(receipt,"before")
            require(self.gateway() == receipt["before_gateway"], "unapplied Gateway baseline changed")
        elif phase in ("awaiting-business-acceptance","accepted","restored","environment-applied"):
            direction = "restored" if phase == "restored" else "after"
            self.exact_env(receipt,direction)
            require(self.gateway() == receipt[direction+"_gateway"], "known Gateway identity changed")
        elif phase in ("applying","restoring","applied-unhealthy"):
            restoring = phase == "restoring"
            direction = "restored" if restoring else "after"
            self.exact_env(receipt,direction)
            current = self.gateway()
            previous = receipt["restore_before_gateway" if restoring else "before_gateway"]
            require(current["id"] != previous["id"] and current["image_id"] == previous["image_id"] and
                    current["image"] == "henukit-console-gateway:"+self.args.sha,
                    "Gateway outcome remains uncertain; no retry or configuration write performed")
            if restoring:
                require(self.healthy(current), "restored Gateway is not healthy; outcome still unresolved")
                receipt.update({"phase":"restored","restored_gateway":current,"business_accepted":False})
            else:
                receipt.update({"phase":"awaiting-business-acceptance" if self.healthy(current) else "applied-unhealthy",
                                "after_gateway":current})
            self.save(receipt)
        else:
            raise RuntimeError("Notice URL operation needs explicit environment reconciliation; no action replayed")
        print(json.dumps({"phase":receipt["phase"],"release_sha":self.args.sha,
                          "business_accepted":receipt["business_accepted"]}))

    def restore(self):
        receipt = self.load()
        if receipt["phase"] == "restored":
            self.exact_env(receipt,"restored")
            require(self.gateway() == receipt["restored_gateway"], "restored Gateway identity changed")
            print(json.dumps({"phase":"restored","release_sha":self.args.sha})); return
        if receipt["phase"] == "restore-environment-applied":
            self.exact_env(receipt,"restored")
            gateway = self.gateway()
            require(gateway == receipt["restore_before_gateway"], "Gateway changed before restore continuation")
            receipt["phase"] = "restoring"; self.save(receipt)
            self.finish_restore(receipt,gateway); return
        require(receipt["phase"] in ("awaiting-business-acceptance","accepted","applied-unhealthy","environment-applied"),
                "Notice URL operation outcome is uncertain; reconcile before restoring")
        data,meta = self.exact_env(receipt,"after")
        gateway = self.gateway()
        require(all(gateway[k] == receipt["after_gateway"][k] for k in ("id","image","image_id","service")),
                "Gateway changed since URL activation; recovery authority is stale")
        lines,index,_,value = url_line(data)
        require(index == receipt["line_index"] and value == URL.encode(), "Notice URL candidate line changed")
        original = base64.b64decode(receipt["original_line_b64"],validate=True)
        require(url_line(original)[3] == b"", "Notice URL preimage is not the original explicit empty value")
        lines[index] = original; restored = b"".join(lines)
        require(sha(restored) == receipt["before_sha256"], "Notice URL restoration would change unrelated bytes")
        temporary,restored_meta = self.replacement(restored,meta)
        receipt.update({"restore_origin_phase":"awaiting-business-acceptance" if receipt["phase"] == "accepted" else receipt["phase"],
                        "phase":"restore-prepared","restore_before_gateway":gateway,"prepared_file":str(temporary),
                        "business_accepted":False,
                        "restored_sha256":sha(restored),"restored_metadata":restored_meta})
        self.save(receipt)
        self.image_proof(gateway)
        self.lock_proof()
        require(read(self.env,private=True) == (data,meta), "Notice URL environment changed before restore")
        self.lock_proof()
        os.replace(temporary,self.env); sync(self.env.parent)
        receipt["phase"] = "restoring"; self.save(receipt)
        self.finish_restore(receipt,gateway)

    def finish_restore(self, receipt, gateway):
        self.recreate(gateway)
        after = self.gateway()
        require(self.healthy(after) and after["id"] != gateway["id"] and after["image_id"] == gateway["image_id"],
                "Gateway restoration not proven; reconcile Notice URL operation")
        receipt.update({"phase":"restored","restored_gateway":after,"business_accepted":False})
        self.save(receipt)
        print(json.dumps({"phase":"restored","release_sha":self.args.sha}))

    def accept(self):
        receipt = self.load()
        require(receipt["phase"] in ("awaiting-business-acceptance","accepted"),
                "business acceptance requires confirmed healthy Gateway activation")
        self.exact_env(receipt,"after")
        require(self.gateway() == receipt["after_gateway"] and self.healthy(receipt["after_gateway"]),
                "Gateway changed before manual business acceptance")
        require(self.args.evidence, "manual authenticated GET evidence is required")
        evidence_data,_ = read(Path(self.args.evidence),private=True)
        evidence = json.loads(evidence_data)
        require(set(evidence) == {"format","release_sha","plan_sha256","route","method","session","result","checked_at"}
                and evidence["format"] == "henukit-notice-business-acceptance-v1"
                and evidence["release_sha"] == self.args.sha and evidence["plan_sha256"] == self.plan_hash
                and evidence["route"] == "/api/v1/notices" and evidence["method"] == "GET"
                and evidence["session"] == "normal-authorized-browser"
                and evidence["result"] == "valid-owner-snapshot",
                "business acceptance must declare the exact real authenticated Notice GET; health alone is insufficient")
        checked_at = datetime.fromisoformat(evidence["checked_at"].replace("Z","+00:00"))
        require(checked_at.tzinfo is not None, "business acceptance time must include its timezone")
        evidence_hash = sha(evidence_data)
        if receipt["phase"] == "accepted":
            require(receipt["business_evidence_sha256"] == evidence_hash,
                    "business acceptance already records different evidence")
        else:
            receipt.update({"phase":"accepted","business_accepted":True,
                            "business_evidence_sha256":evidence_hash,"business_checked_at":evidence["checked_at"]})
            self.save(receipt)
        print(json.dumps({"phase":"accepted","release_sha":self.args.sha,
                          "business_accepted":True,"evidence_source":"manual-browser-observation"}))

    def stage(self):
        require(self.args.baseline_env, "full activation must provide its original private environment backup")
        self.begin(staging=True)

    def health(self):
        receipt = self.load()
        require(receipt["phase"] == "env-staged", "full activation Notice URL stage is not pending")
        self.exact_env(receipt,"after")
        gateway = self.gateway()
        require(self.healthy(gateway) and gateway["id"] != receipt["before_gateway"]["id"],
                "full activation did not establish the exact healthy Gateway")
        receipt.update({"phase":"awaiting-business-acceptance","after_gateway":gateway})
        self.save(receipt)
        print(json.dumps({"phase":receipt["phase"],"release_sha":self.args.sha,"business_accepted":False}))

    def apply(self):
        require(self.active == self.args.sha, "configuration-only apply requires the exact already-active SHA")
        if self.receipt_path.exists():
            receipt = self.load()
            if receipt["phase"] == "not-applied":
                self.exact_env(receipt,"before")
                require(self.gateway() == receipt["before_gateway"], "reconciled unapplied runtime changed")
                previous,_ = read(self.receipt_path,private=True)
                archive = self.receipt_dir/("reconciled-"+sha(previous)+".json")
                if archive.exists() or archive.is_symlink():
                    require(read(archive,private=True)[0] == previous, "prior reconciled attempt audit changed")
                else:
                    self.lock_proof()
                    fd = os.open(archive,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o400)
                    with os.fdopen(fd,"wb") as stream:
                        stream.write(previous); stream.flush(); os.fsync(stream.fileno())
                    sync(self.receipt_dir)
                self.begin(staging=False,reconciled=sha(previous)); return
            require(receipt["phase"] in ("awaiting-business-acceptance","accepted"),
                    "existing Notice URL operation requires reconciliation")
            self.reconcile(); return
        self.begin(staging=False)

    def begin(self, *, staging, reconciled=None):
        if self.receipt_path.exists() or self.receipt_path.is_symlink():
            previous,_ = read(self.receipt_path,private=True)
            require(reconciled == sha(previous) and json.loads(previous)["phase"] == "not-applied",
                    "existing Notice URL operation requires reconciliation")
        for name in ("rollback-contracts/pending", "approvals"):
            directory = self.state/name
            if directory.exists():
                trusted(directory,directory=True,private=True)
                require(not any(p.is_file() and not (staging and name == "approvals" and p.name == self.args.sha)
                                for p in directory.iterdir()), "pending full release must reconcile first")
        data, before = read(self.env,private=True)
        if staging:
            backup = Path(self.args.baseline_env)
            require(backup.parent == self.env.parent and re.fullmatch(r"\.henukit-env\.backup\.[1-9][0-9]*",backup.name),
                    "full activation requires the original sibling environment backup, not the live env")
            baseline,baseline_meta = read(backup,private=True)
            require(sha(baseline) == self.plan["environment_sha256"] and
                    all(before[k] == baseline_meta[k] == self.plan["environment_metadata"][k]
                        for k in ("uid","gid","mode")) and url_line(baseline)[2] == url_line(data)[2],
                    "original full-release environment backup does not bind this Notice URL preimage")
        else:
            require(before == self.plan["environment_metadata"] and sha(data) == self.plan["environment_sha256"],
                    "Notice URL plan environment preimage changed")
        lines,index,line,value = url_line(data)
        require(value == b"", "Notice URL apply requires the explicit empty preimage")
        newline = b"\r\n" if line.endswith(b"\r\n") else b"\n" if line.endswith(b"\n") else b""
        lines[index] = KEY+b"="+URL.encode()+newline
        candidate = b"".join(lines)
        before_gateway = self.gateway()
        require(self.healthy(before_gateway,self.active), "active Gateway is not a healthy exact-SHA baseline")
        self.command([*self.compose_command(),"config","--quiet"])
        self.image_proof(before_gateway)
        temporary,after_meta = self.replacement(candidate,before)
        receipt = {"format":FORMAT,"plan_sha256":self.plan_hash,"release_sha":self.args.sha,
                   "environment_path":str(self.env),"before_metadata":before,
                   "before_sha256":sha(data),"after_metadata":after_meta,"after_sha256":sha(candidate),
                   "original_line_b64":base64.b64encode(line).decode(),"line_index":index,
                   "compose_sha256":self.compose_hash,"before_gateway":before_gateway,
                   "before_active_sha":self.active,"activation_mode":"full" if staging else "configuration",
                   "prepared_file":str(temporary),"reconciled_attempt_sha256":reconciled,
                   "phase":"prepared","business_accepted":False}
        self.save(receipt)
        self.image_proof(before_gateway)
        self.lock_proof()
        require(read(self.env,private=True) == (data,before), "Notice URL environment changed before replacement")
        self.lock_proof()
        os.replace(temporary,self.env); sync(self.env.parent)
        receipt["phase"] = "env-staged" if staging else "applying"; self.save(receipt)
        if staging:
            print(json.dumps({"phase":"env-staged","release_sha":self.args.sha,"business_accepted":False})); return
        self.recreate(before_gateway)
        after_gateway = self.gateway()
        require(self.healthy(after_gateway) and after_gateway["id"] != before_gateway["id"] and
                after_gateway["image_id"] == before_gateway["image_id"],
                "Gateway config reread not proven; reconcile Notice URL operation")
        receipt["after_gateway"] = after_gateway
        receipt["phase"] = "awaiting-business-acceptance"; self.save(receipt)
        print(json.dumps({"phase":receipt["phase"],"release_sha":self.args.sha,"business_accepted":False}))

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("mode","sha","env-file","plan"):
        parser.add_argument("--"+name,required=True)
    for name in ("state-root","release-root","inner","outer","source-lock"):
        parser.add_argument("--"+name)
    parser.add_argument("--evidence"); parser.add_argument("--baseline-env")
    args = parser.parse_args()
    if args.mode == "preflight":
        # Read-only preimage check before the existing activator creates its
        # private backup and makes its unrelated Account updates. Stage later
        # binds the actual copied backup under all four original locks.
        data,meta = read(Path(args.env_file),private=True)
        plan_data,_ = read(Path(args.plan),private=True); plan = json.loads(plan_data)
        require(plan["format"] == "henukit-notice-url-plan-v1" and plan["release_sha"] == args.sha and
                plan["environment_path"] == args.env_file and plan["environment_metadata"] == meta and
                plan["environment_sha256"] == sha(data) and plan["expected_value"] == "" and
                plan["value"] == URL and url_line(data)[3] == b"", "Notice URL preflight plan differs from exact environment")
        return
    require(args.mode in ("apply","restore","reconcile","accept","stage","health"), "unsupported Notice URL action")
    require(all((args.state_root,args.release_root,args.inner,args.outer,args.source_lock)), "original owner authority arguments are required")
    # Match the original artifact helper's required interpolation inputs. Only
    # console-gateway is ever recreated; the Portal placeholders are validation
    # inputs for the other services, never new Portal runtime configuration.
    os.environ["RELEASE_SHA"] = args.sha
    os.environ["PORTAL_VERSION"] = args.sha
    os.environ["PORTAL_DEPLOYED_AT"] = "1970-01-01T00:00:00Z"
    os.environ.pop("NOTICE_API_URL",None)  # the reviewed env-file is authoritative
    transaction = Transaction(args)
    getattr(transaction,args.mode)()

if __name__ == "__main__":
    try:
        main()
    except (RuntimeError,OSError,ValueError,KeyError,TypeError) as error:
        # No subprocess output, env values, credentials, or arbitrary data.
        print("Notice URL transaction refused: "+str(error) if isinstance(error,RuntimeError)
              else "Notice URL transaction refused: authority or state unavailable",file=sys.stderr)
        raise SystemExit(1)
