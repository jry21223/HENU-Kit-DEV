DROP TRIGGER IF EXISTS library_adapter_audit_append_only ON library_adapter_audit_events;
DROP FUNCTION IF EXISTS library_adapter_append_only();
DROP TABLE IF EXISTS library_adapter_audit_events;
DROP TABLE IF EXISTS library_adapter_operations;
