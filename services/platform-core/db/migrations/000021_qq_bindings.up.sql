-- QQ identity is app-scoped. Provision app/client pairs out of band;
-- registering an OAuth client alone never grants Bot authority.
CREATE TABLE IF NOT EXISTS qq_binding_apps (
 app_id text PRIMARY KEY CHECK (length(app_id) BETWEEN 1 AND 128),
 client_id text NOT NULL UNIQUE REFERENCES oauth_clients(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS qq_bindings (
 app_id text NOT NULL REFERENCES qq_binding_apps(app_id) ON DELETE CASCADE,
 subject text NOT NULL CHECK (length(subject) BETWEEN 1 AND 128),
 user_id uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(app_id, subject)
);
CREATE TABLE IF NOT EXISTS qq_binding_challenges (
 token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash)=32),
 token_ciphertext bytea NOT NULL,
 app_id text NOT NULL REFERENCES qq_binding_apps(app_id) ON DELETE CASCADE,
 subject text NOT NULL,
 request_id uuid NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','authorized','confirmed','cancelled')),
 user_id uuid REFERENCES users(id) ON DELETE CASCADE,
 session_id uuid REFERENCES sessions(id) ON DELETE CASCADE,
 expires_at timestamptz NOT NULL DEFAULT now()+interval '5 minutes',
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(app_id, subject, request_id)
);
CREATE INDEX IF NOT EXISTS qq_binding_challenges_expiry ON qq_binding_challenges(expires_at);
CREATE INDEX IF NOT EXISTS qq_binding_challenges_user ON qq_binding_challenges(user_id);
CREATE INDEX IF NOT EXISTS qq_binding_challenges_session ON qq_binding_challenges(session_id);
CREATE TABLE IF NOT EXISTS qq_binding_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 app_id text NOT NULL,
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 action text NOT NULL CHECK (action IN ('bound','unlinked')),
 created_at timestamptz NOT NULL DEFAULT now()
);
