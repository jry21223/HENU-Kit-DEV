BEGIN;
LOCK TABLE quizcraft_legacy_feedback_archives IN ACCESS EXCLUSIVE MODE;
LOCK TABLE quizcraft_migration_exception_resolutions IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM quizcraft_legacy_feedback_archives) THEN
        RAISE EXCEPTION 'cannot remove legacy feedback archive compatibility while archived feedback exists';
    END IF;
    IF EXISTS (
        SELECT 1 FROM quizcraft_migration_exception_resolutions
        WHERE resolution = 'archive_preserved'
    ) THEN
        RAISE EXCEPTION 'cannot remove legacy feedback archive compatibility while archive resolutions exist';
    END IF;
END $$;
ALTER TABLE quizcraft_migration_exception_resolutions
    DROP CONSTRAINT quizcraft_migration_exception_resolutions_resolution_check;
ALTER TABLE quizcraft_migration_exception_resolutions
    ADD CONSTRAINT quizcraft_migration_exception_resolutions_resolution_check
    CHECK (resolution = 'reference_resolved');
DROP TABLE IF EXISTS quizcraft_legacy_feedback_archives;
COMMIT;
