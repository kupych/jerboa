CREATE TABLE events (
    id          BIGSERIAL PRIMARY KEY,
    session_id  TEXT NOT NULL,
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    kind        TEXT NOT NULL,
    path        TEXT NOT NULL DEFAULT '',
    metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_events_session_id ON events(session_id, created_at);
CREATE INDEX idx_events_user_id ON events(user_id, created_at DESC);
CREATE INDEX idx_events_created_at ON events(created_at DESC);
CREATE INDEX idx_events_kind ON events(kind);
