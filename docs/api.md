# V2 API

Base path: `/api/v1`

Currently implemented endpoints:

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
- `GET /api/v1/courses/:id/questions`
- `GET /api/v1/materials?courseId=`
- `GET /api/v1/materials/:id`
- `GET /api/v1/materials/:id/download`
- `GET /api/v1/questions/:id`
- `POST /api/v1/questions/:id/submit`
- `GET /api/v1/me/wrong-questions`
- `DELETE /api/v1/me/wrong-questions/:id`
- `GET /api/v1/me/weakness-report`

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

Implemented material behavior:

- material detail and list responses do not expose `storage_key`
- `free` materials can be downloaded without login
- `login_required` materials require an authenticated, email-verified user
- `paid` materials require an authenticated, email-verified user and a valid `material_access_grants` row for that material
- unsafe or missing storage keys return `file_not_found` without revealing local paths

Implemented quiz behavior:

- question list/detail responses do not expose answers
- submit returns correctness, score, and explanation
- unauthenticated users can submit practice answers, but wrong questions are not persisted
- authenticated wrong answers create or update user-scoped wrong-question records
- weak-point reporting currently returns per-course wrong-count totals
