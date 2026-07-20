BEGIN;
LOCK TABLE quizcraft_banks IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM quizcraft_banks
        WHERE bank_key !~ '^[a-z0-9][a-z0-9-]{1,78}[a-z0-9]$'
    ) THEN
        RAISE EXCEPTION 'cannot remove legacy underscore bank-key compatibility while such banks exist';
    END IF;
END $$;
ALTER TABLE quizcraft_banks DROP CONSTRAINT quizcraft_banks_bank_key_check;
ALTER TABLE quizcraft_banks ADD CONSTRAINT quizcraft_banks_bank_key_check
    CHECK (bank_key ~ '^[a-z0-9][a-z0-9-]{1,78}[a-z0-9]$');
COMMIT;
