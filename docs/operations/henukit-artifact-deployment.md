# HENU Kit fixed-artifact deployment

HENU Kit production is deployed from the Docker images and runtime tarball built by GitHub Actions. The host does not compile the repository and does not read secrets from the release artifact.

## Release procedure

1. Select one successful `Build HENU Kit release artifacts` run for the exact full SHA. Download every image archive, its `.sha256` file, and `henukit-runtime-<sha>.tar.gz` with its checksum.
2. Verify each checksum before loading anything:

   ```bash
   sha256sum -c henukit-<image>-<sha>.docker.tar.gz.sha256
   sha256sum -c henukit-runtime-<sha>.tar.gz.sha256
   docker load < henukit-<image>-<sha>.docker.tar.gz
   ```

3. Extract the runtime tarball into a new release directory. Keep the existing `.env.henukit` outside that directory and make a read-only backup of the `platform` database before applying a migration.
4. If the release contains a not-yet-applied Platform Core migration, pass its exact numbered filename to the release helper. The helper validates the file is inside the artifact, applies it with `ON_ERROR_STOP`, then starts the fixed-SHA Compose file with `--remove-orphans`:

   ```bash
   /opt/henukit-releases/<sha>/bin/deploy-henukit-artifact.sh \
     /opt/henukit-releases/<sha> \
     /opt/henukit/.env.henukit \
     000013_password_registration.up.sql
   ```

   Omit the third argument when the release has no database migration. The helper never runs `docker build`.

5. Verify the active release and public routes. The primary runtime keeps the `quizcraft` and `study` databases for Portal `/practice` and `/library`, but it must not keep the standalone `quizcraft-api`, `quizcraft-web`, `study-api`, or `study-worker` containers:

   ```bash
   docker compose --env-file /opt/henukit/.env.henukit \
     -f /opt/henukit-releases/<sha>/docker-compose.henukit.release.yml ps
   curl --fail --silent --show-error https://superhuazai.me/
   curl --fail --silent --show-error https://superhuazai.me/practice
   curl --fail --silent --show-error https://superhuazai.me/library
   test "$(curl -s -o /dev/null -w '%{http_code}' https://superhuazai.me/quiz/)" = 404
   test "$(curl -s -o /dev/null -w '%{http_code}' https://superhuazai.me/study-api/healthz)" = 404
   ```

Do not delete PostgreSQL databases, Docker volumes, or the retained QuizCraft/Study data as part of this runtime retirement. `--remove-orphans` removes old service containers only; any container that remains causes the helper to fail closed.
