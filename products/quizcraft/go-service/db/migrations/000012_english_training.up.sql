-- Existing sessions remain ordinary; the purpose is fixed at creation.
ALTER TABLE quizcraft_practice_sessions ADD COLUMN IF NOT EXISTS purpose text NOT NULL DEFAULT 'ordinary' CHECK(purpose IN ('ordinary','training'));
CREATE TABLE IF NOT EXISTS quizcraft_training_units (
 id uuid PRIMARY KEY, bank_id uuid NOT NULL UNIQUE REFERENCES quizcraft_banks(id), source_unit_id text NOT NULL, UNIQUE(id,bank_id)
);
CREATE TABLE IF NOT EXISTS quizcraft_training_unit_versions (
 id uuid PRIMARY KEY, unit_id uuid NOT NULL,
 bank_id uuid NOT NULL, bank_version_id uuid NOT NULL UNIQUE,
 content_sha256 text NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 document jsonb NOT NULL, UNIQUE(id,bank_id,bank_version_id),
 FOREIGN KEY(unit_id,bank_id) REFERENCES quizcraft_training_units(id,bank_id),
 FOREIGN KEY(bank_id,bank_version_id) REFERENCES quizcraft_bank_versions(bank_id,id)
);
CREATE TABLE IF NOT EXISTS quizcraft_training_members (
 unit_version_id uuid NOT NULL,bank_id uuid NOT NULL,bank_version_id uuid NOT NULL,
 question_id uuid NOT NULL,question_version_id uuid NOT NULL,position integer NOT NULL CHECK(position>0),
 PRIMARY KEY(unit_version_id,question_id),UNIQUE(unit_version_id,position),
 FOREIGN KEY(unit_version_id,bank_id,bank_version_id) REFERENCES quizcraft_training_unit_versions(id,bank_id,bank_version_id),
 FOREIGN KEY(bank_id,bank_version_id,question_id,question_version_id) REFERENCES quizcraft_bank_version_questions(bank_id,bank_version_id,question_id,question_version_id)
);
CREATE TABLE IF NOT EXISTS quizcraft_training_sessions (
 session_id uuid PRIMARY KEY,unit_version_id uuid NOT NULL,bank_id uuid NOT NULL,bank_version_id uuid NOT NULL,
 FOREIGN KEY(session_id,bank_id,bank_version_id) REFERENCES quizcraft_practice_sessions(id,bank_id,bank_version_id),
 FOREIGN KEY(unit_version_id,bank_id,bank_version_id) REFERENCES quizcraft_training_unit_versions(id,bank_id,bank_version_id)
);
CREATE TABLE IF NOT EXISTS quizcraft_training_outcomes (
 attempt_id uuid PRIMARY KEY REFERENCES quizcraft_practice_attempts(id), assisted boolean NOT NULL,
 outcome text NOT NULL CHECK(outcome IN ('independent_correct','assisted_completion','review_required')),
 review_required boolean NOT NULL
);
CREATE TABLE IF NOT EXISTS quizcraft_training_disclosures (
 id uuid PRIMARY KEY, session_id uuid NOT NULL REFERENCES quizcraft_training_sessions(session_id),
 kind text NOT NULL CHECK(kind IN ('hint','english','chinese')), target_id text NOT NULL DEFAULT '',
 level integer NOT NULL DEFAULT 0, affected_question_ids uuid[] NOT NULL DEFAULT '{}',
 disclosed_at timestamptz NOT NULL DEFAULT now(), UNIQUE(session_id,kind,target_id,level)
);
DO $$ DECLARE relation text; BEGIN
 FOREACH relation IN ARRAY ARRAY['quizcraft_training_units','quizcraft_training_unit_versions','quizcraft_training_members','quizcraft_training_sessions','quizcraft_training_outcomes','quizcraft_training_disclosures'] LOOP
  IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname=relation||'_immutable') THEN
   EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation()',relation||'_immutable',relation);
  END IF;
 END LOOP;
END $$;
-- All ordinary projections share this exclusion, including settlement windows.
CREATE OR REPLACE VIEW quizcraft_ordinary_attempts AS
 SELECT a.* FROM quizcraft_practice_attempts a JOIN quizcraft_practice_sessions s ON s.id=a.session_id WHERE s.purpose='ordinary';
-- Training correctness is factual; mastery/review uses independent completion.
CREATE OR REPLACE VIEW quizcraft_learning_attempt_facts AS
 SELECT a.id,a.bank_id,a.question_id,a.question_version_id,a.submitted_at,
 CASE WHEN s.purpose='training' THEN coalesce(a.user_id,c.user_id) ELSE a.user_id END AS user_id,
 CASE WHEN s.purpose='training' THEN NOT o.review_required ELSE a.correct END AS correct
 FROM quizcraft_practice_attempts a JOIN quizcraft_practice_sessions s ON s.id=a.session_id
 LEFT JOIN quizcraft_practice_session_claims c ON c.session_id=s.id
 LEFT JOIN quizcraft_training_outcomes o ON o.attempt_id=a.id
 WHERE s.purpose='ordinary' OR o.attempt_id IS NOT NULL;
-- One derived rule is shared by live training updates, repair and reconciliation.
CREATE OR REPLACE VIEW quizcraft_expected_learning_state AS
 WITH aggregates AS (
 SELECT user_id,bank_id,question_id,count(*)::bigint AS attempt_count,
 count(*) FILTER(WHERE correct)::bigint AS correct_count,max(submitted_at) AS updated_at
 FROM quizcraft_learning_attempt_facts WHERE user_id IS NOT NULL GROUP BY user_id,bank_id,question_id
 ), latest AS (
 SELECT DISTINCT ON(user_id,bank_id,question_id) user_id,bank_id,question_id,question_version_id,NOT correct AS wrong,id AS latest_attempt_id
 FROM quizcraft_learning_attempt_facts WHERE user_id IS NOT NULL ORDER BY user_id,bank_id,question_id,submitted_at DESC,id DESC
 ) SELECT a.user_id,a.bank_id,a.question_id,l.question_version_id,l.wrong,a.attempt_count,a.correct_count,l.latest_attempt_id,a.updated_at
 FROM aggregates a JOIN latest l USING(user_id,bank_id,question_id);
-- Deferred checks make purpose, exact training binding and outcome atomic even
-- when a caller bypasses HTTP. They also detect orphan training attempts.
CREATE OR REPLACE FUNCTION quizcraft_guard_training_session() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE session uuid; purpose text; binding boolean;
BEGIN
 IF TG_TABLE_NAME='quizcraft_practice_sessions' THEN session:=NEW.id; ELSE session:=NEW.session_id; END IF;
 SELECT s.purpose,EXISTS(SELECT 1 FROM quizcraft_training_sessions t WHERE t.session_id=s.id) INTO purpose,binding FROM quizcraft_practice_sessions s WHERE s.id=session;
 IF (purpose='training') IS DISTINCT FROM binding THEN RAISE EXCEPTION 'training purpose and exact unit binding must be created atomically'; END IF;
 IF purpose='training' AND EXISTS(
 SELECT 1 FROM quizcraft_training_sessions t WHERE t.session_id=session AND (
 EXISTS(SELECT question_id,question_version_id,position FROM quizcraft_training_members m WHERE m.unit_version_id=t.unit_version_id EXCEPT SELECT question_id,question_version_id,position FROM quizcraft_practice_session_questions q WHERE q.session_id=session)
 OR EXISTS(SELECT question_id,question_version_id,position FROM quizcraft_practice_session_questions q WHERE q.session_id=session EXCEPT SELECT question_id,question_version_id,position FROM quizcraft_training_members m WHERE m.unit_version_id=t.unit_version_id))) THEN RAISE EXCEPTION 'training session must pin the whole exact ordered unit membership'; END IF;
 IF purpose='ordinary' AND EXISTS(SELECT 1 FROM quizcraft_practice_sessions s JOIN quizcraft_training_units u ON u.bank_id=s.bank_id WHERE s.id=session) THEN RAISE EXCEPTION 'ordinary sessions cannot use training banks'; END IF;
 RETURN NULL;
END $$;
CREATE OR REPLACE FUNCTION quizcraft_guard_training_attempt() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE attempt uuid; training boolean; valid boolean;
BEGIN
 IF TG_TABLE_NAME='quizcraft_practice_attempts' THEN attempt:=NEW.id; ELSE attempt:=NEW.attempt_id; END IF;
 SELECT s.purpose='training',
 CASE WHEN o.attempt_id IS NULL THEN false
 ELSE (o.outcome=CASE WHEN NOT a.correct THEN 'review_required' WHEN o.assisted THEN 'assisted_completion' ELSE 'independent_correct' END AND o.review_required=(NOT a.correct OR o.assisted)) END
 INTO training,valid FROM quizcraft_practice_attempts a JOIN quizcraft_practice_sessions s ON s.id=a.session_id LEFT JOIN quizcraft_training_outcomes o ON o.attempt_id=a.id WHERE a.id=attempt;
 IF training AND NOT valid THEN RAISE EXCEPTION 'training attempt requires a consistent immutable outcome'; END IF;
 IF NOT training AND EXISTS(SELECT 1 FROM quizcraft_training_outcomes WHERE attempt_id=attempt) THEN RAISE EXCEPTION 'ordinary attempt cannot have a training outcome'; END IF;
 RETURN NULL;
END $$;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='quizcraft_training_session_consistent') THEN
 CREATE CONSTRAINT TRIGGER quizcraft_training_session_consistent AFTER INSERT ON quizcraft_practice_sessions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION quizcraft_guard_training_session();
 CREATE CONSTRAINT TRIGGER quizcraft_training_binding_consistent AFTER INSERT ON quizcraft_training_sessions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION quizcraft_guard_training_session();
 CREATE CONSTRAINT TRIGGER quizcraft_training_membership_consistent AFTER INSERT ON quizcraft_practice_session_questions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION quizcraft_guard_training_session();
 CREATE CONSTRAINT TRIGGER quizcraft_training_attempt_consistent AFTER INSERT ON quizcraft_practice_attempts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION quizcraft_guard_training_attempt();
 CREATE CONSTRAINT TRIGGER quizcraft_training_outcome_consistent AFTER INSERT ON quizcraft_training_outcomes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION quizcraft_guard_training_attempt();
 END IF;
END $$;
