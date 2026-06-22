# API Seed

Run from `services/api` after PostgreSQL is available:

```bash
go run ./cmd/seed
```

The seed is safe to run repeatedly and creates demo data for local development:

- 河南大学 / 软件学院
- 网络工程、软件工程
- 2023级 / 2024级 demo courses
- published materials with local-storage keys
- single choice, multiple choice, true/false, fill blank, and short answer questions
- demo users: `admin@example.com`, `reviewer@example.com`, `creator@example.com`, `user@example.com`
- wiki, blog, forum, moment, points, membership, notification, and mock AI task examples

The seed does not create or commit real course files. Material `storage_key` values point under `uploads/materials/...`; provide local files separately when testing downloads.
