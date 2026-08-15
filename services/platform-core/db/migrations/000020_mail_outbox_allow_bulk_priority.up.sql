-- Career Opportunities digest mail (#404): 000019 generalized mail_outbox.kind
-- for career_digest rows but the verification-bound priority CHECK still only
-- admits 'critical', so the digest enqueue (priority 'bulk') violates the table
-- constraint. Relax it to admit the bulk job-digest class; the worker claim
-- query already orders non-critical priorities as a single normal class.

ALTER TABLE mail_outbox
    DROP CONSTRAINT IF EXISTS mail_outbox_priority_check;

ALTER TABLE mail_outbox
    ADD CONSTRAINT mail_outbox_priority_check
    CHECK (priority IN ('critical', 'bulk'));
