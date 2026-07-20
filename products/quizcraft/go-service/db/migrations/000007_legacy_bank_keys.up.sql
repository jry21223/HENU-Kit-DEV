ALTER TABLE quizcraft_banks DROP CONSTRAINT quizcraft_banks_bank_key_check;
ALTER TABLE quizcraft_banks ADD CONSTRAINT quizcraft_banks_bank_key_check
    CHECK (bank_key ~ '^[a-z0-9][a-z0-9_-]{1,78}[a-z0-9]$');
