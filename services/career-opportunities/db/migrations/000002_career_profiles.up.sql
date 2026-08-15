-- Career Opportunities: user career profile (issue #394).
--
-- The profile is Career-owned (never Account Portfolio) and only a Lifetime
-- member may read or write it. No resume file, recruitment account password,
-- or cookie is ever stored: the fields are constrained text and enums sized so
-- an arbitrarily large blob cannot enter the crawler/matcher.

CREATE TABLE IF NOT EXISTS career_profiles (
    user_id uuid PRIMARY KEY,
    target_roles text NOT NULL DEFAULT '' CHECK (char_length(target_roles) <= 500),
    tech_stack text NOT NULL DEFAULT '' CHECK (char_length(tech_stack) <= 1000),
    locations text NOT NULL DEFAULT '' CHECK (char_length(locations) <= 500),
    job_type text NOT NULL DEFAULT '' CHECK (job_type IN ('','daily_intern','summer_intern','campus_recruit')),
    graduation_year integer CHECK (graduation_year IS NULL OR (graduation_year BETWEEN 1900 AND 2200)),
    resume_text text NOT NULL DEFAULT '' CHECK (char_length(resume_text) <= 4000),
    email_notification_enabled boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now()
);
