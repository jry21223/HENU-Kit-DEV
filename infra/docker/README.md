# Docker Support

Dockerfiles live beside each app/service:

- `services/api/Dockerfile`
- `services/worker/Dockerfile`
- `apps/web/Dockerfile`
- `apps/admin/Dockerfile`

Compose files:

- `docker-compose.yml` and `docker-compose.dev.yml`: local development stack.
- `docker-compose.prod.example.yml`: production-like example stack using private Postgres/Redis, API, Worker, Web, Admin, and Nginx.

Important production notes:

- Copy `.env.production.example` to `.env.production`; never commit `.env.production`.
- `NEXT_PUBLIC_API_BASE_URL` and `VITE_API_BASE_URL` are build-time values. Rebuild Web/Admin after changing public domains.
- API and Worker containers expose readiness probes for Docker healthchecks: API `/readyz` on port 8080, Worker `/readyz` on `WORKER_HEALTH_PORT` (default 9090).
- The API image currently ships the server binary only. Seed/import jobs should run from a trusted workstation or future dedicated job image.
- Real uploads, JWT keys, WeChat Pay keys, WeChat certificates, TLS certificates, and database dumps are ignored by Git.
