CREATE TABLE IF NOT EXISTS users (
    id           BIGSERIAL   PRIMARY KEY,
    username     TEXT        NOT NULL UNIQUE
                             CHECK (username ~ '^[a-z0-9_]{3,20}$'),
    display_name TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);