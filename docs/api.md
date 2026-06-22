# V2 API

Base path: `/api/v1`

Stage 1 skeleton endpoints:

- `GET /healthz`
- `GET /api/v1/healthz`
- `GET /api/v1/version`

Response envelope:

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

Error envelope:

```json
{
  "code": 40001,
  "message": "unauthorized",
  "details": {}
}
```

Later stages add auth, organization, course, material, quiz, AI, points, membership, wiki, social, notification, report, and admin APIs.
