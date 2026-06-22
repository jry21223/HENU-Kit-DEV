# V2 API

Base path: `/api/v1`

Stage 1 skeleton endpoints:

- `GET /healthz`
- `GET /api/v1/healthz`
- `GET /api/v1/version`
- `POST /api/v1/auth/send-code`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `GET /api/v1/schools`
- `GET /api/v1/colleges?schoolId=`
- `GET /api/v1/majors?schoolId=&collegeId=`
- `GET /api/v1/courses?schoolId=&majorId=&grade=`
- `GET /api/v1/courses/:id`
- `GET /api/v1/courses/:id/materials`

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

Later stages add full organization, course, material, quiz, AI, points, membership, wiki, social, notification, report, and admin APIs.

Implemented authentication behavior:

- Verification codes are hashed before storage.
- Development and test can use `DEV_FIXED_VERIFICATION_CODE=123456`.
- Production does not return codes in API responses.
- JWT uses RS256. Production must provide a private key through environment or a mounted file.
- `access_token` and `refresh_token` are issued as httpOnly cookies; the access token is also returned for API clients.
