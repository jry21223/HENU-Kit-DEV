-- Career Opportunities: async resume extraction jobs (resume upload → AI
-- extraction → profile prefill).
--
-- An extraction is created queued with the uploaded file bytes held
-- transiently; a background worker claims it, advances
-- queued -> running -> completed (or failed with a stable browser-safe error
-- code), and purges the file bytes once processed. Only the extracted text
-- fields remain durable: the promise "只保存文字信息，不存储简历文件" holds
-- end to end. No `lifetime` flag is stored here: Career never becomes a second
-- membership truth source.

CREATE TABLE IF NOT EXISTS career_resume_extractions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('queued','running','completed','failed')),
    file_name text NOT NULL CHECK (char_length(file_name) <= 255),
    file_sha256 text NOT NULL CHECK (char_length(file_sha256) = 64),
    file_content bytea,
    extracted jsonb,
    error_code text,
    error_message text,
    started_at timestamptz,
    completed_at timestamptz,
    failed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_career_resume_extractions_user_created ON career_resume_extractions (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_career_resume_extractions_queued ON career_resume_extractions (status) WHERE status = 'queued';
