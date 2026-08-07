-- Portal notice read permission (ADR-0013: portal.* codes are isolated from
-- console.*). The Portal Gateway checks portal.notice.read at product scope
-- "portal" before any signed Notice owner read. Students read distributed
-- notices, so the permission is granted to the canonical student role from
-- docs/architecture/ACCESS_CONTROL.md; per-user grants are created at runtime
-- through Platform Operations (product scope "portal").
INSERT INTO permission_codes (code, description, status) VALUES
    ('portal.notice.read', 'Read distributed notices within Portal product Scope', 'active')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, status = 'active';

INSERT INTO authorization_roles (code, display_name, status)
VALUES ('student', 'Student', 'active')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_code)
SELECT id, 'portal.notice.read' FROM authorization_roles WHERE code = 'student'
ON CONFLICT (role_id, permission_code) DO NOTHING;
