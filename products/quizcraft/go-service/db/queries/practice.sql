-- name: GetOperationResult :one
SELECT resource_id, response_body
FROM quizcraft_idempotency_results
WHERE actor_key = $1 AND operation_kind = $2 AND idempotency_key = $3;

-- name: ListPublishedBanks :many
SELECT b.id, b.active_version_id, b.bank_key, b.name, bv.content_sha256,
       count(m.question_id)::bigint AS question_count,
       COALESCE(jsonb_agg(DISTINCT jsonb_build_object('id',qv.chapter_id,'name',qv.chapter_name)) FILTER (WHERE qv.id IS NOT NULL),'[]'::jsonb) AS chapters
FROM quizcraft_banks b
JOIN quizcraft_bank_versions bv ON bv.id=b.active_version_id AND bv.bank_id=b.id AND bv.sealed_at IS NOT NULL
LEFT JOIN quizcraft_bank_version_questions m ON m.bank_version_id=bv.id
LEFT JOIN quizcraft_question_versions qv ON qv.id=m.question_version_id
GROUP BY b.id,b.active_version_id,b.bank_key,b.name,bv.content_sha256
ORDER BY b.name,b.id;

-- name: IsPublishedBankVersion :one
SELECT true AS published
FROM quizcraft_banks b
JOIN quizcraft_bank_versions bv ON bv.id=$2 AND bv.bank_id=b.id
WHERE b.id=$1 AND b.active_version_id=$2 AND bv.sealed_at IS NOT NULL;

-- name: CreatePracticeSession :exec
INSERT INTO quizcraft_practice_sessions(id,bank_id,bank_version_id,user_id,actor_key,mode,chapter_id)
VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: AddPracticeSessionQuestion :exec
INSERT INTO quizcraft_practice_session_questions(session_id,bank_id,bank_version_id,question_id,question_version_id,position)
VALUES($1,$2,$3,$4,$5,$6);

-- name: ListPracticeQuestions :many
SELECT m.question_id,m.question_version_id,qv.type,qv.chapter_id,qv.chapter_name,qv.content,COALESCE(qv.options,'null'::jsonb) AS options
FROM quizcraft_bank_version_questions m
JOIN quizcraft_question_versions qv ON qv.id=m.question_version_id AND qv.question_id=m.question_id AND qv.bank_id=m.bank_id
LEFT JOIN quizcraft_question_stats stats ON stats.question_id=m.question_id
WHERE m.bank_version_id=sqlc.arg(bank_version_id) AND (sqlc.arg(chapter_id)::text='' OR qv.chapter_id=sqlc.arg(chapter_id) OR qv.chapter_name=sqlc.arg(chapter_id))
ORDER BY
  CASE WHEN sqlc.arg(difficult)::boolean THEN CASE WHEN COALESCE(stats.attempt_count,0)=0 THEN 1 ELSE stats.correct_count::numeric/stats.attempt_count END END ASC,
  md5(m.question_id::text || sqlc.arg(session_id)::uuid::text)
LIMIT sqlc.arg(question_count);

-- name: LockSubmission :exec
SELECT pg_advisory_xact_lock(hashtextextended($1,0));

-- name: GetSessionQuestion :one
SELECT s.bank_id,s.bank_version_id,COALESCE(c.user_id,s.user_id) AS user_id,COALESCE(c.user_actor_key,s.actor_key) AS actor_key,qv.type,qv.answer,COALESCE(qv.options,'null'::jsonb) AS options,qv.analysis,b.bank_key,q.source_question_id
FROM quizcraft_practice_sessions s
JOIN quizcraft_practice_session_questions sq ON sq.session_id=s.id
JOIN quizcraft_question_versions qv ON qv.id=sq.question_version_id AND qv.question_id=sq.question_id
JOIN quizcraft_questions q ON q.id=sq.question_id
JOIN quizcraft_banks b ON b.id=s.bank_id
LEFT JOIN quizcraft_practice_session_claims c ON c.session_id=s.id
WHERE s.id=$1 AND sq.question_id=$2 AND sq.question_version_id=$3;

-- name: ClaimGuestPracticeSession :execrows
INSERT INTO quizcraft_practice_session_claims(session_id,guest_actor_key,user_id,user_actor_key)
SELECT id,$2,$3,$4
FROM quizcraft_practice_sessions
WHERE id=$1 AND user_id IS NULL AND actor_key=$2
ON CONFLICT(session_id) DO NOTHING;

-- name: GetPracticeAttemptResponse :one
SELECT response_body
FROM quizcraft_practice_attempts
WHERE session_id=$1 AND question_id=$2;

-- name: CreatePracticeAttempt :exec
INSERT INTO quizcraft_practice_attempts(id,session_id,bank_id,bank_version_id,question_id,question_version_id,user_id,submitted_answer,correct,expected_answer,analysis,response_body)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12);

-- name: UpdateQuestionStats :exec
INSERT INTO quizcraft_question_stats(question_id,attempt_count,correct_count)
VALUES($1,1,$2)
ON CONFLICT(question_id) DO UPDATE
SET attempt_count=quizcraft_question_stats.attempt_count+1,
    correct_count=quizcraft_question_stats.correct_count+EXCLUDED.correct_count,
    updated_at=now();

-- name: UpdateLearningState :exec
INSERT INTO quizcraft_learning_state(user_id,bank_id,question_id,question_version_id,wrong,attempt_count,correct_count)
VALUES($1,$2,$3,$4,$5,1,$6)
ON CONFLICT(user_id,bank_id,question_id) DO UPDATE
SET question_version_id=EXCLUDED.question_version_id,
    wrong=EXCLUDED.wrong,
    attempt_count=quizcraft_learning_state.attempt_count+1,
    correct_count=quizcraft_learning_state.correct_count+EXCLUDED.correct_count,
    updated_at=now();

-- name: ListLearningState :many
SELECT bank_id,question_id,question_version_id,wrong,attempt_count,correct_count,updated_at
FROM quizcraft_learning_state
WHERE user_id=$1
ORDER BY updated_at DESC;

-- name: InsertShadowComparison :exec
INSERT INTO quizcraft_shadow_comparisons(id,session_id,question_id,new_response,legacy_response,outcome,detail)
VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: LockIdempotency :exec
SELECT pg_advisory_xact_lock(hashtextextended($1,0));

-- name: GetIdempotencyResult :one
SELECT request_sha256,response_status,response_body
FROM quizcraft_idempotency_results
WHERE actor_key=$1 AND operation_kind=$2 AND idempotency_key=$3;

-- name: StoreIdempotencyResult :exec
INSERT INTO quizcraft_idempotency_results(actor_key,operation_kind,idempotency_key,request_sha256,response_status,response_body,resource_id)
VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: LockFavorite :exec
SELECT pg_advisory_xact_lock(hashtextextended($1,0));

-- name: IsQuestionInBank :one
SELECT true AS exists
FROM quizcraft_questions
WHERE bank_id=$1 AND id=$2;

-- name: AddFavorite :exec
INSERT INTO quizcraft_favorites(user_id,bank_id,question_id)
VALUES($1,$2,$3)
ON CONFLICT(user_id,bank_id,question_id) DO NOTHING;

-- name: RemoveFavorite :exec
DELETE FROM quizcraft_favorites
WHERE user_id=$1 AND bank_id=$2 AND question_id=$3;

-- name: ListFavoriteFolders :many
SELECT f.bank_id,b.name AS bank_name,
       count(*) FILTER (WHERE m.question_id IS NOT NULL)::bigint AS available_count,
       count(*) FILTER (WHERE m.question_id IS NULL)::bigint AS unavailable_count
FROM quizcraft_favorites f
JOIN quizcraft_banks b ON b.id=f.bank_id
LEFT JOIN quizcraft_bank_version_questions m
  ON m.bank_id=f.bank_id AND m.question_id=f.question_id AND m.bank_version_id=b.active_version_id
WHERE f.user_id=$1
GROUP BY f.bank_id,b.name
ORDER BY b.name,f.bank_id;

-- name: ListFavoriteQuestions :many
SELECT f.bank_id,f.question_id,m.question_version_id
FROM quizcraft_favorites f
JOIN quizcraft_banks b ON b.id=f.bank_id
LEFT JOIN quizcraft_bank_version_questions m
  ON m.bank_id=f.bank_id AND m.question_id=f.question_id AND m.bank_version_id=b.active_version_id
WHERE f.user_id=$1 AND f.bank_id=$2
ORDER BY f.created_at,f.question_id;

-- name: GetActiveBankVersion :one
SELECT bv.id
FROM quizcraft_banks b
JOIN quizcraft_bank_versions bv ON bv.id=b.active_version_id AND bv.bank_id=b.id AND bv.sealed_at IS NOT NULL
WHERE b.id=$1;

-- name: ListFavoritePracticeQuestions :many
SELECT m.question_id,m.question_version_id,qv.type,qv.chapter_id,qv.chapter_name,qv.content,COALESCE(qv.options,'null'::jsonb) AS options
FROM quizcraft_favorites f
JOIN quizcraft_bank_version_questions m
  ON m.bank_id=f.bank_id AND m.question_id=f.question_id AND m.bank_version_id=sqlc.arg(bank_version_id)
JOIN quizcraft_question_versions qv
  ON qv.id=m.question_version_id AND qv.question_id=m.question_id AND qv.bank_id=m.bank_id
WHERE f.user_id=sqlc.arg(user_id) AND f.bank_id=sqlc.arg(bank_id)
ORDER BY f.created_at,f.question_id;

-- name: CountFavoritesForBank :one
SELECT count(*)::bigint
FROM quizcraft_favorites
WHERE user_id=$1 AND bank_id=$2;
