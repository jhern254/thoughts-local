-- Tags
CREATE TABLE IF NOT EXISTS tags (
    tag_id     INTEGER PRIMARY KEY,
    user_id    TEXT    NOT NULL,
    tag_name       TEXT    NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (unixepoch('now')),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch('now')),
    version    INTEGER NOT NULL DEFAULT 1 CHECK (version > 0), -- optimistic-lock version

    -- Validation checks
    CONSTRAINT ck_tags_tag_id_len
        CHECK (length(trim(tag_id)) BETWEEN 1 AND 255),

    CONSTRAINT ck_tags_name_len
        CHECK (length(trim(tag_name)) BETWEEN 1 AND 4096),

    CONSTRAINT ck_tags_time_order
        CHECK (created_at <= updated_at),

    -- Foreign key
    CONSTRAINT fk_tags_user
        FOREIGN KEY (user_id)
        REFERENCES users(user_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE
);

-- Per-user uniqueness of tag names
CREATE UNIQUE INDEX IF NOT EXISTS uq_tags_user_name
    ON tags (user_id, tag_name);

CREATE INDEX IF NOT EXISTS idx_tags_user
    ON tags (user_id);
