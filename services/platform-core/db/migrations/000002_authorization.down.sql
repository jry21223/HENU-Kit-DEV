DROP TABLE IF EXISTS authorization_audit_events;
DROP TABLE IF EXISTS user_role_grants;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS authorization_roles;
DROP TABLE IF EXISTS permission_codes;

ALTER TABLE IF EXISTS users DROP COLUMN IF EXISTS authorization_revision;
