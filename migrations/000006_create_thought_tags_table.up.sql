-- Thought ↔ Tag join table
CREATE TABLE IF NOT EXISTS thought_tags (
    thought_id  INTEGER NOT NULL,
    tag_id      INTEGER NOT NULL,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- link audit (epoch seconds UTC)

    -- Composite primary key
    CONSTRAINT pk_thought_tags
        PRIMARY KEY (thought_id, tag_id),

    -- Foreign keys
    CONSTRAINT fk_thought_tags_thought
        FOREIGN KEY (thought_id) REFERENCES thoughts(thought_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE,

    CONSTRAINT fk_thought_tags_tag
        FOREIGN KEY (tag_id) REFERENCES tags(tag_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE
);

-- Index to support queries like: "all thoughts for a tag"
CREATE INDEX IF NOT EXISTS idx_thought_tags_tag
    ON thought_tags (tag_id);
