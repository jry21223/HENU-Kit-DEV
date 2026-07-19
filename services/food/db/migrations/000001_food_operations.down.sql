DROP TRIGGER IF EXISTS food_audit_append_only ON food_audit_events;
DROP FUNCTION IF EXISTS food_audit_append_only();
DROP TABLE IF EXISTS food_audit_events;
DROP TABLE IF EXISTS food_operations;
DROP TABLE IF EXISTS food_tier_adjustments;
DROP TABLE IF EXISTS food_anomaly_tickets;
DROP TABLE IF EXISTS food_submissions;
