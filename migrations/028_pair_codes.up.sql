CREATE TABLE pair_codes (
    code         TEXT PRIMARY KEY,
    user_id      UUID REFERENCES users(id) ON DELETE CASCADE,
    band_id      UUID REFERENCES bands(id) ON DELETE CASCADE,
    expires_at   TIMESTAMPTZ NOT NULL,
    consumed_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_pair_codes_expires_at ON pair_codes(expires_at);
