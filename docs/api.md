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
- `GET /api/v1/courses/:id/packages`
- `GET /api/v1/courses/:id/questions`
- `GET /api/v1/materials?courseId=`
- `GET /api/v1/materials/:id`
- `GET /api/v1/materials/:id/download`
- `GET /api/v1/packages?courseId=&schoolId=&majorId=&grade=`
- `GET /api/v1/packages/:id`
- `GET /api/v1/questions/:id`
- `POST /api/v1/questions/:id/submit`
- `POST /api/v1/quiz/attempts`
- `GET /api/v1/me/quiz-attempts`
- `GET /api/v1/me/wrong-questions`
- `DELETE /api/v1/me/wrong-questions/:id`
- `GET /api/v1/me/weakness-report`
- `GET /api/v1/me/downloads`
- `POST /api/v1/ai/tasks`
- `GET /api/v1/ai/tasks/:id`
- `POST /api/v1/admin/schools`
- `PATCH /api/v1/admin/schools/:id`
- `DELETE /api/v1/admin/schools/:id`
- `POST /api/v1/admin/colleges`
- `PATCH /api/v1/admin/colleges/:id`
- `DELETE /api/v1/admin/colleges/:id`
- `POST /api/v1/admin/majors`
- `PATCH /api/v1/admin/majors/:id`
- `DELETE /api/v1/admin/majors/:id`
- `GET /api/v1/admin/courses?schoolId=&majorId=&grade=&status=`
- `POST /api/v1/admin/courses`
- `PATCH /api/v1/admin/courses/:id`
- `DELETE /api/v1/admin/courses/:id`
- `GET /api/v1/admin/materials?courseId=&status=`
- `POST /api/v1/admin/materials`
- `PATCH /api/v1/admin/materials/:id/status`
- `PATCH /api/v1/admin/materials/:id`
- `DELETE /api/v1/admin/materials/:id`
- `POST /api/v1/admin/materials/upload`
- `GET /api/v1/admin/downloads?materialId=&userId=`
- `GET /api/v1/admin/ai/tasks`
- `GET /api/v1/admin/ai/drafts`
- `POST /api/v1/admin/ai/drafts/:id/approve`
- `POST /api/v1/admin/ai/drafts/:id/reject`
- `GET /api/v1/admin/analytics/overview`

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
- `paid` materials require an authenticated, email-verified user and either a valid material grant or a valid package grant containing that material
- successful downloads create `material_download_logs` records with material, optional user, access level, IP, user agent, and download time
- denied downloads, unsafe storage keys, and missing files are not recorded as successful downloads
- logged-in users can list only their own successful downloads through `/me/downloads`
- admin users can list successful download audit logs, including IP and User-Agent metadata
- unsafe or missing storage keys return `file_not_found` without revealing local paths

Implemented package behavior:

- package list/detail endpoints only return `published` course packages
- package detail returns package items plus published materials included in the package
- a `material_access_grants.package_id` grant unlocks paid package materials on the server side
- expired package grants do not unlock paid materials

Implemented quiz behavior:

- question list/detail responses do not expose answers
- submit returns correctness, score, and explanation
- unauthenticated users can submit practice answers, but wrong questions are not persisted
- logged-in users can create quiz attempts and list only their own attempts
- authenticated wrong answers create or update user-scoped wrong-question records
- weak-point reporting currently returns per-course wrong-count totals

Implemented admin behavior:

- all admin endpoints require an authenticated `admin` or `super_admin` role
- organization/course/material delete operations archive by setting `status=archived`
- admin course list returns all course statuses; public course list/detail returns only `published`
- course create/update accepts only `draft`, `published`, or `archived`
- admin material list returns all material statuses; public material list/detail returns only `published`
- material create/upload defaults to `draft` when status is omitted
- material status updates accept only `draft`, `pending`, `published`, or `archived`
- material upload uses server-generated storage keys under `materials/{courseId}/`
- upload accepts only `.pdf`, `.txt`, `.md`, and `.docx`; PDFs must start with a PDF header
- upload rejects files larger than 20 MiB
- manually supplied `storageKey` values with path traversal are rejected
- admin analytics overview returns read-only totals, 14-day successful-download trend, top materials, course demand, and access-level breakdown

Implemented AI behavior:

- logged-in users can create AI tasks and query only their own tasks
- supported task types are `chat`, `wrong_question_analysis`, `targeted_question`, `paper_generation`, and `draft_review`
- admin users can list AI tasks and AI drafts
- Redis Stream enqueue is best-effort; database task creation remains the source of truth
- worker mock mode turns pending tasks into pending AI drafts
- approving a draft marks the draft reviewed but does not publish generated content automatically
- rejecting a draft marks the draft rejected and does not delete the source task or generated content
- the Vue admin `/ai/drafts` page is a UI wrapper over these admin-only AI review endpoints
