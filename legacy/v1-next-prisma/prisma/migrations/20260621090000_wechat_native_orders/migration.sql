ALTER TYPE "order_status" ADD VALUE IF NOT EXISTS 'paying';
ALTER TYPE "order_status" ADD VALUE IF NOT EXISTS 'closed';
ALTER TYPE "order_status" ADD VALUE IF NOT EXISTS 'expired';

ALTER TABLE "orders"
ADD COLUMN IF NOT EXISTS "amount_total" INTEGER NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS "currency" TEXT NOT NULL DEFAULT 'CNY',
ADD COLUMN IF NOT EXISTS "payment_provider" TEXT NOT NULL DEFAULT 'wechat_native',
ADD COLUMN IF NOT EXISTS "out_trade_no" TEXT,
ADD COLUMN IF NOT EXISTS "transaction_id" TEXT,
ADD COLUMN IF NOT EXISTS "wx_trade_state" TEXT,
ADD COLUMN IF NOT EXISTS "code_url" TEXT,
ADD COLUMN IF NOT EXISTS "expires_at" TIMESTAMP(3),
ADD COLUMN IF NOT EXISTS "raw_notify" JSONB,
ADD COLUMN IF NOT EXISTS "risk_flag" BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN IF NOT EXISTS "updated_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE UNIQUE INDEX IF NOT EXISTS "orders_out_trade_no_key" ON "orders"("out_trade_no");
CREATE UNIQUE INDEX IF NOT EXISTS "orders_transaction_id_key" ON "orders"("transaction_id");
CREATE INDEX IF NOT EXISTS "orders_payment_provider_status_idx" ON "orders"("payment_provider", "status");
