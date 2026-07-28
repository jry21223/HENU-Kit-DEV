CREATE TABLE IF NOT EXISTS account_portfolio_point_adjustment_audits (
    id UUID PRIMARY KEY,
    operator_user_id UUID NOT NULL,
    target_user_id UUID NOT NULL REFERENCES account_portfolio_accounts(user_id) ON DELETE RESTRICT,
    amount BIGINT NOT NULL CHECK (amount BETWEEN -9007199254740991 AND 9007199254740991 AND amount <> 0),
    reason TEXT NOT NULL CHECK (length(reason) BETWEEN 1 AND 1000),
    idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 8 AND 200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS account_portfolio_point_adjustment_audits_target_created_idx
    ON account_portfolio_point_adjustment_audits (target_user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS account_portfolio_point_adjustment_audits_operator_created_idx
    ON account_portfolio_point_adjustment_audits (operator_user_id, created_at DESC, id DESC);

ALTER TABLE account_portfolio_point_ledger
    ADD COLUMN IF NOT EXISTS audit_id UUID REFERENCES account_portfolio_point_adjustment_audits(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX IF NOT EXISTS account_portfolio_point_ledger_audit_unique_idx
    ON account_portfolio_point_ledger (audit_id)
    WHERE audit_id IS NOT NULL;

ALTER TABLE account_portfolio_notifications
    ADD COLUMN IF NOT EXISTS point_ledger_id UUID REFERENCES account_portfolio_point_ledger(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX IF NOT EXISTS account_portfolio_notifications_point_ledger_unique_idx
    ON account_portfolio_notifications (point_ledger_id)
    WHERE point_ledger_id IS NOT NULL;

-- HC-167 did not expose a point mutation API, but preserve any nonzero
-- projection left by a manual pre-rollout import as one immutable opening fact
-- rather than silently changing the user's real balance.
INSERT INTO account_portfolio_point_ledger(id, user_id, amount, reason, idempotency_key, created_at)
SELECT
    (
        substr(md5('account-portfolio-points-opening:' || p.user_id::text), 1, 8) || '-' ||
        substr(md5('account-portfolio-points-opening:' || p.user_id::text), 9, 4) || '-' ||
        substr(md5('account-portfolio-points-opening:' || p.user_id::text), 13, 4) || '-' ||
        substr(md5('account-portfolio-points-opening:' || p.user_id::text), 17, 4) || '-' ||
        substr(md5('account-portfolio-points-opening:' || p.user_id::text), 21, 12)
    )::uuid,
    p.user_id,
    p.balance,
    'Legacy balance reconciliation',
    'legacy_points_reconciliation:' || p.user_id::text,
    p.updated_at
FROM account_portfolio_points p
WHERE p.balance > 0
  AND NOT EXISTS (
      SELECT 1
      FROM account_portfolio_point_ledger l
      WHERE l.user_id = p.user_id
  )
ON CONFLICT (id) DO NOTHING;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM account_portfolio_point_ledger
        GROUP BY user_id
        HAVING COALESCE(SUM(amount), 0) < 0
    ) THEN
        RAISE EXCEPTION 'cannot reconcile a negative Account Portfolio point ledger balance';
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM account_portfolio_point_ledger
        WHERE amount < -9007199254740991 OR amount > 9007199254740991
    ) OR EXISTS (
        SELECT 1
        FROM account_portfolio_point_ledger
        GROUP BY user_id
        HAVING COALESCE(SUM(amount), 0) > 9007199254740991
    ) THEN
        RAISE EXCEPTION 'cannot reconcile an Account Portfolio point ledger outside the supported JSON-safe range';
    END IF;
END $$;

ALTER TABLE account_portfolio_point_ledger
    DROP CONSTRAINT IF EXISTS account_portfolio_point_ledger_json_safe_amount;
ALTER TABLE account_portfolio_point_ledger
    ADD CONSTRAINT account_portfolio_point_ledger_json_safe_amount
    CHECK (amount BETWEEN -9007199254740991 AND 9007199254740991);

ALTER TABLE account_portfolio_points
    DROP CONSTRAINT IF EXISTS account_portfolio_points_json_safe_balance;
ALTER TABLE account_portfolio_points
    ADD CONSTRAINT account_portfolio_points_json_safe_balance
    CHECK (balance BETWEEN 0 AND 9007199254740991);

-- The projection is kept for row-level serialization and summary joins. The
-- ledger is the source of truth and the owner recomputes it for every read.
UPDATE account_portfolio_points p
SET balance = COALESCE((
        SELECT SUM(l.amount)
        FROM account_portfolio_point_ledger l
        WHERE l.user_id = p.user_id
    ), 0),
    updated_at = now();

CREATE OR REPLACE FUNCTION account_portfolio_reject_point_fact_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'Account Portfolio point ledger and adjustment audits are immutable';
END;
$$;

DROP TRIGGER IF EXISTS account_portfolio_point_ledger_immutable ON account_portfolio_point_ledger;
CREATE TRIGGER account_portfolio_point_ledger_immutable
    BEFORE UPDATE OR DELETE ON account_portfolio_point_ledger
    FOR EACH ROW EXECUTE FUNCTION account_portfolio_reject_point_fact_mutation();

DROP TRIGGER IF EXISTS account_portfolio_point_adjustment_audits_immutable ON account_portfolio_point_adjustment_audits;
CREATE TRIGGER account_portfolio_point_adjustment_audits_immutable
    BEFORE UPDATE OR DELETE ON account_portfolio_point_adjustment_audits
    FOR EACH ROW EXECUTE FUNCTION account_portfolio_reject_point_fact_mutation();
