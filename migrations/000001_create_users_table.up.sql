-- Users table.
CREATE TABLE IF NOT EXISTS users (
    user_id    TEXT PRIMARY KEY,
    handle     TEXT    UNIQUE,
    alt_handle TEXT,        
    email      TEXT    UNIQUE,
    version    INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- epoch seconds (UTC)
    updated_at INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- epoch seconds (UTC)
    deleted_at INTEGER, -- NULL means undeleted; UTC epoch seconds otherwise

    -- Validation checks
    -- auth
    CONSTRAINT ck_users_user_id_len
        CHECK (length(trim(user_id)) BETWEEN 1 AND 255),

    -- If provided, handle/email must not be empty/whitespace and must be within a sane max.
    CONSTRAINT ck_users_handle_len
        CHECK (handle IS NULL OR length(trim(handle)) BETWEEN 1 AND 512),

    CONSTRAINT ck_users_alt_handle_len
        CHECK (alt_handle IS NULL OR length(trim(alt_handle)) BETWEEN 1 AND 512),

    -- Timestamps must be logical
    CONSTRAINT ck_users_deleted_at
        CHECK (deleted_at IS NULL OR
            (typeof(deleted_at) = 'integer' AND created_at <= deleted_at AND deleted_at <= updated_at)),

    CONSTRAINT ck_users_time_order
        CHECK (created_at <= updated_at)
);
