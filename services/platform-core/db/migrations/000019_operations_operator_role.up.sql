INSERT INTO authorization_roles(code, display_name, status)
VALUES ('operations-operator', 'Operations operator', 'active')
ON CONFLICT (code) DO NOTHING;
