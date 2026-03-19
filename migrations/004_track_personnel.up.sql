CREATE TABLE track_personnel (
    track_id    UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (track_id, user_id)
);
