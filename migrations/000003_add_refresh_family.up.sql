ALTER TABLE refresh_tokens ADD COLUMN family_id uuid NOT NULL DEFAULT gen_random_uuid();
CREATE INDEX idx_refresh_tokens_family_id ON refresh_tokens (family_id);
