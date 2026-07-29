-- A DO block makes the lock/rebuild atomic both for the versioned Go runner
-- (which already wraps each source in a transaction) and for direct psql -f
-- recovery drills, where otherwise LOCK TABLE would be rejected outside an
-- explicit transaction block. Match RebuildLearningState's lock order so no
-- submission can insert an immutable attempt between the fact scan and the
-- replacement projection.
DO $$
BEGIN
  LOCK TABLE quizcraft_practice_attempts IN SHARE MODE;
  LOCK TABLE quizcraft_learning_state IN ACCESS EXCLUSIVE MODE;

  ALTER TABLE quizcraft_learning_state
      ADD COLUMN IF NOT EXISTS latest_attempt_id uuid;

  -- Learning state is a derived projection. Rebuild it from immutable attempts
  -- so pre-existing rows receive the same deterministic latest-attempt key used
  -- by live writes and reconciliation.
  DELETE FROM quizcraft_learning_state;

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
           user_id,bank_id,question_id,id AS latest_attempt_id,question_version_id,(NOT correct) AS wrong
    FROM quizcraft_practice_attempts
    WHERE user_id IS NOT NULL
    ORDER BY user_id,bank_id,question_id,submitted_at DESC,id DESC
  )
  INSERT INTO quizcraft_learning_state(user_id,bank_id,question_id,question_version_id,wrong,attempt_count,correct_count,latest_attempt_id,updated_at)
  SELECT aggregates.user_id,aggregates.bank_id,aggregates.question_id,latest.question_version_id,
         latest.wrong,aggregates.attempt_count,aggregates.correct_count,latest.latest_attempt_id,aggregates.updated_at
  FROM aggregates
  JOIN latest USING (user_id,bank_id,question_id);

  ALTER TABLE quizcraft_learning_state
      ALTER COLUMN latest_attempt_id SET NOT NULL;

  BEGIN
    ALTER TABLE quizcraft_learning_state
        ADD CONSTRAINT quizcraft_learning_state_latest_attempt_fk
        FOREIGN KEY(latest_attempt_id) REFERENCES quizcraft_practice_attempts(id) ON DELETE RESTRICT;
  EXCEPTION WHEN duplicate_object THEN NULL;
  END;
END $$;
