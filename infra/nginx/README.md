# Nginx

`final-review.conf.example` is a production-style reverse-proxy template.

`henukit-host.conf.example` is the host-side canonical entry for
`henukit.cn`. It terminates TLS, redirects `www` with HTTP 308, applies the
HENU Kit 32 MB edge limit, and proxies the single public origin to the Compose
edge on `127.0.0.1:8088`. Keep the legacy host in a separate enabled vhost
until the rollback window closes.

It expects:

- `review.example.com` for the student-facing Web app.
- `admin.review.example.com` for the Vue Admin app.
- `/api/` on both domains proxied to the Go API.
- TLS certificates mounted at `/etc/nginx/certs/<domain>/fullchain.pem` and `/etc/nginx/certs/<domain>/privkey.pem`.

Before use:

1. Replace `review.example.com` and `admin.review.example.com`.
2. Mount real TLS certificates from ignored `certs/tls/`.
3. Confirm `CORS_ALLOWED_ORIGINS` in `.env.production` exactly matches the HTTPS origins.
4. Run `docker compose --env-file .env.production -f docker-compose.prod.example.yml config --quiet`.

`final-review.conf.example` includes baseline security headers and a 25 MB
upload limit matching the Final Review API upload limit. The HENU Kit host
template intentionally keeps its independently verified 32 MB production edge
contract.

Current hardening in `final-review.conf.example`:

- HTTP to HTTPS redirect, with ACME challenge pass-through.
- HSTS, CSP, frame denial, content-type sniffing protection, referrer policy, permissions policy, and cross-origin opener policy.
- `proxy_cookie_flags` to mark API cookies `Secure`, `HttpOnly`, and `SameSite=Lax` at the edge.
- 25 MB request body limit and explicit proxy timeouts for API/Web/Admin traffic.
- hidden dotfile requests are denied except `/.well-known`.
- `/uploads/` requests return 404 on both public domains. Runtime upload
  volumes must not be mounted into Nginx/Web/Admin as static files; materials
  and moment images are served through Go API endpoints so permission and
  visibility checks always run.

After editing the template, run an `nginx -t` check with real or temporary certificates before deployment. The release checklist in `docs/operations/deployment.md` includes a Docker-based example.
