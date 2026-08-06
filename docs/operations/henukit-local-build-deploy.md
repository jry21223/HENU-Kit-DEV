# Deploying without GitHub Actions artifacts

The supported release path is `docs/operations/henukit-artifact-deployment.md`:
GitHub Actions builds the fixed-SHA images, and production downloads them
directly. **Use that whenever it works.**

This document covers the fallback used when Actions cannot publish artifacts —
in practice when the repository has hit its Actions artifact storage quota, so
every `Upload image artifact` step fails while the build itself is fine:

```
Failed to CreateArtifact: Artifact storage quota has been hit.
Usage is recalculated every 6-12 hours.
```

Deleting artifacts does **not** clear the quota immediately; the counter is
recalculated on GitHub's own schedule. Fixing the quota is the real remedy, and
this fallback should be retired as soon as it is available again.

## What the fallback trades away

Building locally skips the checks CI would have run on the artifact, and that
has already caused a production-shaped bug:

> A `${VAR:-default}` inside a short-form Compose volume (`source:target:mode`)
> has more colons than the spec can hold. Compose 2.x on the CI runner rejects
> it outright. Compose 5.3.1 accepts it and silently renders `type: volume`, so
> production would have mounted an empty anonymous volume and served an empty
> directory with no error anywhere.

So: **push the branch and let CI run even when you deploy from a local build.**
The governance jobs (`branch-name`, `review-evidence`, `release-contract`) and
`portal-responsive` do not upload artifacts and still gate the change. Only the
`image-*` and `runtime-config` upload steps are blocked by the quota.

## Prerequisites on the build machine

- Docker with the base images reachable. If the daemon sits behind a proxy,
  confirm it is actually applied — a proxy configured in
  `/etc/systemd/system/docker.service.d/` needs `systemctl restart docker`
  before `dockerd` picks it up, and until then pulls fail with i/o timeouts.
- Build containers do not inherit the daemon's proxy. Pass it per build:
  `--network=host --build-arg HTTP_PROXY=... --build-arg HTTPS_PROXY=...`,
  otherwise `go mod download` and `pnpm install` time out inside the build.
- Node and pnpm, for the runtime packaging step's boundary check.

## 1. Build the images

Reproduce the matrix in `.github/workflows/deploy-henukit.yml` for the exact
commit being released. The image tag **must** be the full 40-character SHA; the
release Compose file resolves every service through `${RELEASE_SHA}`.

Twelve images are built: `console`, `console-gateway`, `platform-core`,
`platform-mail-worker`, `platform-smtp-provider`, `portal`, `portal-api`,
`account-portfolio`, `notice`, `notice-worker`, `food`, `portal-gateway`.
`portal` and `console` take the `NEXT_PUBLIC_*` / `VITE_*` build args listed in
the workflow — omitting them produces an image that looks fine and behaves
wrongly at runtime.

Verify each image is `amd64` before exporting; a build on an ARM machine
produces images production cannot run.

**Only rebuild what changed.** A commit touching one service needs one image;
re-tag the rest from the previous release rather than rebuilding and
re-transferring them:

```bash
docker tag henukit-<name>:<previous-sha> henukit-<name>:<new-sha>
```

## 2. Package the runtime

Mirror the `runtime-config` job. It renders the release Compose file, copies the
nginx template, systemd units, EasyPay patches and the migrations for
`platform-core`, `account-portfolio`, `notice`, `food` and `portal`, installs
the ops scripts, runs the Account production boundary check, and writes
`RELEASE_SHA`.

Keep this in step with the workflow. A stale copy of this step shipped a bundle
with no `migrations/portal`, which would have left the release without its
schema change and no error until the feature was used.

Confirm before shipping:

```bash
tar tzf henukit-runtime-<sha>.tar.gz | grep migrations/
tar xzOf henukit-runtime-<sha>.tar.gz ./docker-compose.henukit.release.yml | grep -A3 'srv/materials'
```

## 3. Transfer

Know where the bytes are slow before moving ~180 MB. On the setup this was
written for:

| Link | Throughput |
|---|---|
| laptop → production | ~4.8 MB/s |
| build machine → laptop | <60 KB/s |

The build machine sat on a home connection whose *upload* was the constraint —
`tailscale status` showed a `direct` path, so this was bandwidth, not relaying.
Routing artifacts build machine → laptop → production sends everything twice
and puts the slowest hop in the middle.

Prefer, in order:

1. Restore the Actions path so production downloads artifacts itself.
2. Transfer build machine → production directly. SSH agent forwarding does not
   survive a Tailscale SSH hop, so this needs a key the build machine holds.
3. Route through the laptop, and start the transfer to production for each file
   as it lands rather than waiting for the whole set.

Verify every checksum on production before loading anything:

```bash
cd /opt/henukit-releases/<sha>
for f in *.sha256; do sha256sum -c "$f"; done
docker load < henukit-<image>-<sha>.docker.tar.gz
tar xzf henukit-runtime-<sha>.tar.gz
```

## 4. Back up, then activate

Back up every database the release touches. Migrations are applied by the
helper, before the release activates:

```bash
for db in portal platform account_portfolio notice food study; do
  docker exec henukit-postgres-1 pg_dump -U henukit -d "$db" \
    --format=custom -f "/tmp/${db}-$(date +%Y%m%d%H%M%S).dump"
  docker cp "henukit-postgres-1:/tmp/${db}-"*.dump /root/db-backups/
done

/opt/henukit-releases/<sha>/bin/deploy-henukit-artifact.sh \
  /opt/henukit-releases/<sha> \
  /opt/henukit-releases/<env-sha>/.env.henukit
```

The environment file is **not** part of the artifact. Its current location is in
`/etc/henukit/actions-watch.env` as `HENUKIT_ENV_FILE`; read it there rather
than assuming a path.

Pass a third argument only when a Platform Core migration must be applied, as a
comma-separated list of filenames from `migrations/platform-core`.

A new required variable makes the helper fail before it changes anything —
which is the desired behaviour. Add it to the environment file, keeping a dated
copy of the previous version, and re-run.

### Deploying a single service

When only one service changed and the rest are re-tagged, the whole release can
still be activated normally. To move one service alone:

```bash
RELEASE_SHA=<sha> docker compose \
  --env-file <env-file> \
  -f /opt/henukit-releases/<sha>/docker-compose.henukit.release.yml \
  up -d --no-deps <service>
```

This leaves the other containers on their current images. Use it to stage a
verification, not as the normal path — it leaves the deployment mixed-version.

## 5. Verify

```bash
docker compose --env-file <env-file> -f <release-compose> ps
for u in / /food /library /practice /account; do
  echo "$u $(curl -s -o /dev/null -w '%{http_code}' "https://henukit.cn$u")"
done
curl -s -o /dev/null -w '%{http_code}\n' https://console.henukit.cn/

for c in portal-api portal-gateway platform-core food notice; do
  echo "$c $(docker logs "henukit-$c-1" --since 5m 2>&1 | grep -ciE 'error|panic|fatal')"
done
```

Check the specific behaviour the release changed, not just that pages return
200. A release is not verified because nothing crashed.

## Rollback

Previous releases stay under `/opt/henukit-releases/<sha>` with their images
loaded, so rolling back is re-running the helper against the earlier directory.
Restore a database only if the release migrated it; the dumps are in
`/root/db-backups/`.

## After deploying this way

Push the branch, open the PR, and keep `Review-Head` at the deployed SHA. A
locally built release that never reaches `main` leaves production running code
no one can find from the repository.
