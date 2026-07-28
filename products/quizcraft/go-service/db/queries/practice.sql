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

-- name: GetPracticeSessionActor :one
SELECT COALESCE(c.user_actor_key,s.actor_key) AS actor_key
FROM quizcraft_practice_sessions s
LEFT JOIN quizcraft_practice_session_claims c ON c.session_id=s.id
WHERE s.id=$1;

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

-- name: GetLearningStateReconciliation :one
WITH aggregates AS (
  SELECT user_id,bank_id,question_id,
         count(*)::bigint AS attempt_count,
         count(*) FILTER (WHERE correct)::bigint AS correct_count,
         max(submitted_at) AS updated_at
  FROM quizcraft_practice_attempts
  WHERE user_id IS NOT NULL
  GROUP BY user_id,bank_id,question_id
),
latest AS (
  SELECT DISTINCT ON (user_id,bank_id,question_id)
         user_id,bank_id,question_id,question_version_id,(NOT correct) AS wrong
  FROM quizcraft_practice_attempts
  WHERE user_id IS NOT NULL
  ORDER BY user_id,bank_id,question_id,submitted_at DESC,id DESC
),
expected AS (
  SELECT aggregates.user_id,aggregates.bank_id,aggregates.question_id,
         latest.question_version_id,latest.wrong,aggregates.attempt_count,
         aggregates.correct_count,aggregates.updated_at
  FROM aggregates
  JOIN latest USING (user_id,bank_id,question_id)
),
compared AS (
  SELECT expected.user_id AS expected_user_id,
         actual.user_id AS actual_user_id,
         expected.question_version_id AS expected_question_version_id,
         actual.question_version_id AS actual_question_version_id,
         expected.wrong AS expected_wrong,actual.wrong AS actual_wrong,
         expected.attempt_count AS expected_attempt_count,actual.attempt_count AS actual_attempt_count,
         expected.correct_count AS expected_correct_count,actual.correct_count AS actual_correct_count,
         expected.updated_at AS expected_updated_at,actual.updated_at AS actual_updated_at
  FROM expected
  FULL OUTER JOIN quizcraft_learning_state actual
    ON actual.user_id=expected.user_id AND actual.bank_id=expected.bank_id AND actual.question_id=expected.question_id
)
SELECT count(*) FILTER (WHERE expected_user_id IS NOT NULL AND actual_user_id IS NULL)::bigint AS missing_rows,
       count(*) FILTER (WHERE expected_user_id IS NULL AND actual_user_id IS NOT NULL)::bigint AS extra_rows,
       count(*) FILTER (WHERE expected_user_id IS NOT NULL AND actual_user_id IS NOT NULL AND
         (expected_question_version_id IS DISTINCT FROM actual_question_version_id OR
          expected_wrong IS DISTINCT FROM actual_wrong OR
          expected_attempt_count IS DISTINCT FROM actual_attempt_count OR
          expected_correct_count IS DISTINCT FROM actual_correct_count OR
          expected_updated_at IS DISTINCT FROM actual_updated_at))::bigint AS mismatched_rows
FROM compared;

-- name: ClearLearningState :exec
DELETE FROM quizcraft_learning_state;

-- name: RebuildLearningStateFromAttempts :exec
WITH aggregates AS (
  SELECT user_id,bank_id,question_id,
         count(*)::bigint AS attempt_count,
         count(*) FILTER (WHERE correct)::bigint AS correct_count,
         max(submitted_at) AS updated_at
  FROM quizcraft_practice_attempts
  WHERE user_id IS NOT NULL
  GROUP BY user_id,bank_id,question_id
),
latest AS (
  SELECT DISTINCT ON (user_id,bank_id,question_id)
         user_id,bank_id,question_id,question_version_id,(NOT correct) AS wrong
  FROM quizcraft_practice_attempts
  WHERE user_id IS NOT NULL
  ORDER BY user_id,bank_id,question_id,submitted_at DESC,id DESC
)
INSERT INTO quizcraft_learning_state(user_id,bank_id,question_id,question_version_id,wrong,attempt_count,correct_count,updated_at)
SELECT aggregates.user_id,aggregates.bank_id,aggregates.question_id,latest.question_version_id,
       latest.wrong,aggregates.attempt_count,aggregates.correct_count,aggregates.updated_at
FROM aggregates
JOIN latest USING (user_id,bank_id,question_id);

-- name: GetPersonalPracticeTotals :one
SELECT count(*)::bigint AS total_answers,
       count(*) FILTER (WHERE correct)::bigint AS correct_answers
FROM quizcraft_practice_attempts
WHERE user_id=$1;

-- name: ListPersonalPracticeDays :many
SELECT DISTINCT ((submitted_at AT TIME ZONE 'Asia/Shanghai')::date)::text AS activity_day
FROM quizcraft_practice_attempts
WHERE user_id=$1
ORDER BY activity_day DESC;

-- name: ListPersonalMasteryFacts :many
WITH active_questions AS (
  SELECT b.id AS bank_id,b.name,m.question_id
  FROM quizcraft_banks b
  JOIN quizcraft_bank_versions bv ON bv.id=b.active_version_id AND bv.bank_id=b.id AND bv.sealed_at IS NOT NULL
  JOIN quizcraft_bank_version_questions m ON m.bank_id=b.id AND m.bank_version_id=bv.id
),
bank_totals AS (
  SELECT bank_id,name AS label,count(*)::bigint AS total_questions
  FROM active_questions
  GROUP BY bank_id,name
),
personal_activity AS (
  SELECT active.bank_id,
         count(DISTINCT active.question_id) FILTER (WHERE attempt.correct)::bigint AS correct_questions
  FROM active_questions active
  JOIN quizcraft_practice_attempts attempt
    ON attempt.bank_id=active.bank_id AND attempt.question_id=active.question_id
  WHERE attempt.user_id=$1
  GROUP BY active.bank_id
)
SELECT totals.bank_id,totals.label,totals.total_questions,activity.correct_questions
FROM bank_totals totals
JOIN personal_activity activity ON activity.bank_id=totals.bank_id
ORDER BY totals.label,totals.bank_id;

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

-- name: UpsertRankingProfile :exec
INSERT INTO quizcraft_ranking_profiles(user_id,nickname,system_avatar,visible)
VALUES($1,$2,$3,$4)
ON CONFLICT(user_id) DO UPDATE SET nickname=EXCLUDED.nickname,system_avatar=EXCLUDED.system_avatar,visible=EXCLUDED.visible,updated_at=now();

-- name: LockRankingProfileMutation :exec
SELECT pg_advisory_xact_lock(hashtextextended('ranking-profile-user:' || $1::text,0));

-- name: ListOverallRanking :many
WITH totals AS (
  SELECT a.user_id,p.nickname,p.system_avatar,count(*)::bigint AS correct_answer_count
  FROM quizcraft_practice_attempts a
  JOIN quizcraft_ranking_profiles p ON p.user_id=a.user_id AND p.visible
  WHERE a.correct AND a.user_id IS NOT NULL AND a.submitted_at >= $1
  GROUP BY a.user_id,p.nickname,p.system_avatar
)
SELECT rank() OVER (ORDER BY correct_answer_count DESC)::bigint AS rank,nickname,system_avatar,correct_answer_count
FROM totals
ORDER BY correct_answer_count DESC,user_id
LIMIT 100;

-- name: ListBankRanking :many
WITH totals AS (
  SELECT a.user_id,p.nickname,p.system_avatar,count(*)::bigint AS correct_answer_count
  FROM quizcraft_practice_attempts a
  JOIN quizcraft_ranking_profiles p ON p.user_id=a.user_id AND p.visible
  WHERE a.correct AND a.user_id IS NOT NULL AND a.bank_id=$1 AND a.submitted_at >= $2
  GROUP BY a.user_id,p.nickname,p.system_avatar
)
SELECT rank() OVER (ORDER BY correct_answer_count DESC)::bigint AS rank,nickname,system_avatar,correct_answer_count
FROM totals
ORDER BY correct_answer_count DESC,user_id
LIMIT 100;

-- name: ListOverallRankingWindow :many
WITH totals AS (
  SELECT a.user_id,p.nickname,p.system_avatar,count(*)::bigint AS correct_answer_count
  FROM quizcraft_practice_attempts a
  JOIN quizcraft_ranking_profiles p ON p.user_id=a.user_id AND p.visible
  WHERE a.correct AND a.user_id IS NOT NULL AND a.submitted_at >= $1 AND a.submitted_at < $2
  GROUP BY a.user_id,p.nickname,p.system_avatar
)
SELECT user_id,rank() OVER (ORDER BY correct_answer_count DESC)::bigint AS rank,nickname,system_avatar,correct_answer_count
FROM totals ORDER BY correct_answer_count DESC,user_id LIMIT 100;

-- name: ListBankRankingWindow :many
WITH totals AS (
  SELECT a.user_id,p.nickname,p.system_avatar,count(*)::bigint AS correct_answer_count
  FROM quizcraft_practice_attempts a
  JOIN quizcraft_ranking_profiles p ON p.user_id=a.user_id AND p.visible
  WHERE a.correct AND a.user_id IS NOT NULL AND a.bank_id=$1 AND a.submitted_at >= $2 AND a.submitted_at < $3
  GROUP BY a.user_id,p.nickname,p.system_avatar
)
SELECT user_id,rank() OVER (ORDER BY correct_answer_count DESC)::bigint AS rank,nickname,system_avatar,correct_answer_count
FROM totals ORDER BY correct_answer_count DESC,user_id LIMIT 100;

-- name: CreateRankingSettlementEvent :execrows
INSERT INTO quizcraft_ranking_settlement_events(id,period_start,period_end,scope,bank_id,metric,standings)
VALUES($1,$2,$3,$4,$5,'correct_answer_count',$6)
ON CONFLICT(period_start,period_end,scope,bank_id) DO NOTHING;

-- name: ListScoredBankIDsWindow :many
SELECT DISTINCT a.bank_id
FROM quizcraft_practice_attempts a
WHERE a.correct AND a.user_id IS NOT NULL AND a.submitted_at >= $1 AND a.submitted_at < $2
ORDER BY a.bank_id;
