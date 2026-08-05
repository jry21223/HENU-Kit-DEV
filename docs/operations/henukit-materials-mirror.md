# Course materials mirror

The Library serves course materials from a mirror of the public
[HENU-Final-Review](https://github.com/jry21223/HENU-Final-Review) repository.
The repository stays the source of truth; production holds a read-only copy so
students are not sent to `raw.githubusercontent.com`, which is unreliable from
mainland China.

## What is published

`manifest.json` decides. Only assets whose `role` is **not** a `待复核`
(pending review) one are mirrored — the repository's own `PUBLICATION_POLICY.md`
keeps unreviewed material out of the formal directories until a maintainer
confirms its provenance, and the mirror holds that same line.

At the time of writing that is 182 of 248 assets, about 474 MB across 13
subjects; the 66 pending-review assets stay in the repository only.

## Layout on the host

```
/opt/henukit-materials/
├── repo/        git checkout — never served
├── public/      the mirror nginx serves
├── bin/         sync script and webhook receiver
└── SYNCED_SHA   commit the mirror was built from
```

The checkout and the served root are deliberately separate. Serving the
checkout would expose `.git` — the whole repository history — along with the
repository's tooling, so only files named by the manifest are copied across.
The sync refuses to publish if a dotfile ends up in the mirror.

## First install

```bash
install -d /opt/henukit-materials/bin
install -m 0755 scripts/ops/sync-henukit-materials.sh /opt/henukit-materials/bin/
install -m 0755 scripts/ops/henukit-materials-webhook.mjs /opt/henukit-materials/bin/
/opt/henukit-materials/bin/sync-henukit-materials.sh
```

nginx serves the mirror at `/materials/` from the compose bind mount
(`HENUKIT_MATERIALS_ROOT`, default `/opt/henukit-materials/public`). Nothing
under that root is executed, directory listing is off, dotfiles 404, and every
response carries `Content-Disposition: attachment` plus `nosniff`.

Materials are served with a one-day `Cache-Control`. Cloudflare caches these
file extensions by default, which matters here: the origin uplink is about
3 Mbps, so repeat downloads must not reach it.

## Automatic re-mirroring

`henukit-materials-webhook.mjs` re-runs the sync when the repository is pushed.

1. Create the secret and environment file (root-owned, not world readable):

   ```bash
   install -d -m 0700 /etc/henukit
   printf 'HENUKIT_MATERIALS_WEBHOOK_SECRET=%s\n' "$(openssl rand -hex 32)" \
     > /etc/henukit/materials-webhook.env
   chmod 0600 /etc/henukit/materials-webhook.env
   ```

2. Install and start the unit:

   ```bash
   install -m 0644 infra/systemd/henukit-materials-webhook.service /etc/systemd/system/
   systemctl daemon-reload
   systemctl enable --now henukit-materials-webhook
   ```

3. Add the webhook in the HENU-Final-Review repository settings:
   - Payload URL: `https://henukit.cn/webhook/materials`
   - Content type: `application/json`
   - Secret: the value generated above
   - Events: **Just the push event**

The receiver binds to `127.0.0.1:8099`; the host TLS terminator forwards to it,
so the signature is a second gate rather than the only one. It verifies
`X-Hub-Signature-256` in constant time, ignores pushes to other branches, and
answers before the mirror finishes because a 474 MB rebuild takes far longer
than GitHub's delivery timeout. Concurrent deliveries collapse into one run
with at most one queued rerun, since the mirror is rebuilt from the manifest
every time.

## Verifying

```bash
cat /opt/henukit-materials/SYNCED_SHA
find /opt/henukit-materials/public -type f | wc -l
journalctl -u henukit-materials-webhook --since '1 hour ago'

# A published asset downloads; a dotfile and the checkout never do.
curl -sI 'https://henukit.cn/materials/高等数学A（二）/复习讲义/高等数学A（二）_考前复习知识点讲义.pdf' | head -5
test "$(curl -s -o /dev/null -w '%{http_code}' https://henukit.cn/materials/.git/config)" = 404
```

## Rollback

The sync is idempotent and rebuilds from the manifest, so re-running it fixes a
bad mirror. To stop publishing entirely, `systemctl disable --now
henukit-materials-webhook` and remove the `/materials/` location; the Library
loses its download links but nothing else depends on the mirror.
