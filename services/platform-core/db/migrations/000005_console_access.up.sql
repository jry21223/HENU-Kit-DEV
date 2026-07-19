INSERT INTO permission_codes (code, description)
VALUES ('console.overview.read', 'Open HENUKit Console and read the server-verified Overview access context')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, status = 'active';
