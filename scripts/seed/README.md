# Seed Scripts

V2 seed and import commands live in `services/api/cmd`.

```bash
cd services/api
go run ./cmd/seed
go run ./cmd/import-materials ../../data/material-manifest.example.json
```

`cmd/seed` creates repeatable demo data. `cmd/import-materials` imports already-prepared course files that are mounted under `LOCAL_UPLOAD_DIR`; the files themselves are ignored by Git.
