# Deployment

Deployment is not production-ready in the Stage 1 skeleton.

The intended production shape is:

- PostgreSQL managed database or hardened container
- Redis managed service or hardened container
- Go API and Worker containers
- Next.js Web container
- Vue Admin static/container deployment
- Nginx or platform ingress

Production must provide real JWT keys, payment keys, upload storage, and LLM configuration.
