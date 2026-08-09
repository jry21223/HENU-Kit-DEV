DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM authorization_roles AS roles
        JOIN role_permissions AS permissions ON permissions.role_id = roles.id
        WHERE roles.code = 'portal-notice-reader'
          AND roles.status = 'active'
          AND permissions.permission_code <> 'portal.notice.read'
    ) THEN
        RAISE EXCEPTION 'active portal-notice-reader role must have only portal.notice.read';
    END IF;
END;
$$;

INSERT INTO permission_codes (code, description)
VALUES ('portal.notice.read', 'Read the Portal Notice feed within Portal product Scope')
ON CONFLICT (code) DO UPDATE
SET description = EXCLUDED.description
WHERE permission_codes.status = 'active';

INSERT INTO authorization_roles (code, display_name)
VALUES ('portal-notice-reader', 'Portal notice reader')
ON CONFLICT (code) DO UPDATE
SET display_name = EXCLUDED.display_name,
    updated_at = now()
WHERE authorization_roles.status = 'active';

INSERT INTO role_permissions (role_id, permission_code)
SELECT roles.id, permissions.code
FROM authorization_roles AS roles
JOIN permission_codes AS permissions ON permissions.code = 'portal.notice.read' AND permissions.status = 'active'
WHERE roles.code = 'portal-notice-reader'
  AND roles.status = 'active'
ON CONFLICT (role_id, permission_code) DO NOTHING;

INSERT INTO user_role_grants (user_id, role_id, scope_kind, product_code)
SELECT users.id, roles.id, 'product', 'portal'
FROM users
JOIN authorization_roles AS roles ON roles.code = 'portal-notice-reader' AND roles.status = 'active'
JOIN role_permissions AS permissions ON permissions.role_id = roles.id AND permissions.permission_code = 'portal.notice.read'
JOIN permission_codes AS codes ON codes.code = permissions.permission_code AND codes.status = 'active'
WHERE users.status = 'active'
  AND users.email_verified
  AND NOT EXISTS (
      SELECT 1
      FROM role_permissions AS extra_permissions
      WHERE extra_permissions.role_id = roles.id
        AND extra_permissions.permission_code <> 'portal.notice.read'
  )
  AND NOT EXISTS (
      SELECT 1
      FROM user_role_grants AS existing
      WHERE existing.user_id = users.id
        AND existing.role_id = roles.id
        AND existing.scope_kind = 'product'
        AND existing.product_code = 'portal'
  );
