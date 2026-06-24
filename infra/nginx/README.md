# Nginx

`final-review.conf.example` is a production-style reverse-proxy template.

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

The template includes baseline security headers and a 25 MB upload limit matching the current API upload limit.
