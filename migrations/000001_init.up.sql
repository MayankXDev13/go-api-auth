CREATE TABLE users (
    id uuid PRIMARY KEY,
    name varchar(100) NOT NULL,
    email varchar(255) NOT NULL,
    password_hash text NOT NULL,
    created_at timestamptz,
    updated_at timestamptz
);

CREATE UNIQUE INDEX idx_users_email ON users (email);

CREATE TABLE refresh_tokens (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    token_hash varchar(64) NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz
);

CREATE UNIQUE INDEX idx_refresh_tokens_token_hash ON refresh_tokens (token_hash);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id);

CREATE TABLE password_reset_tokens (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    token_hash varchar(64) NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz
);

CREATE UNIQUE INDEX idx_password_reset_tokens_token_hash ON password_reset_tokens (token_hash);
CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens (user_id);

CREATE TABLE todos (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    title varchar(255) NOT NULL,
    description varchar(1000),
    completed boolean NOT NULL DEFAULT false,
    due_date timestamptz,
    created_at timestamptz,
    updated_at timestamptz
);

CREATE INDEX idx_todos_user_id ON todos (user_id);
