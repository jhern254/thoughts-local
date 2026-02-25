-- Goals
CREATE TABLE IF NOT EXISTS goals (
    goal_id          INTEGER PRIMARY KEY,
    user_id          TEXT    NOT NULL,
    goal_name        TEXT    NOT NULL,
    target_seconds   INTEGER NOT NULL,

    -- Store dates as ISO-8601 text: YYYY-MM-DD (nullable)
    goal_start_date  TEXT,
    goal_end_date    TEXT,

    -- SQLite boolean: 0/1
    goal_is_active   INTEGER NOT NULL DEFAULT 1,

    -- Enum-like fields stored as TEXT + CHECK
    cadence          TEXT    NOT NULL DEFAULT 'weekly',
    tz               TEXT    NOT NULL DEFAULT 'UTC',
    week_start       TEXT    NOT NULL DEFAULT 'mon',
    default_cadence  TEXT,

    created_at       INTEGER NOT NULL DEFAULT (unixepoch('now')),
    updated_at       INTEGER NOT NULL DEFAULT (unixepoch('now')),

    -- Validation checks
    CONSTRAINT ck_goals_goal_name_len
        CHECK (length(trim(goal_name)) BETWEEN 1 AND 512),

    CONSTRAINT ck_goals_target_seconds_pos
        CHECK (target_seconds > 0),

    CONSTRAINT ck_goals_is_active_bool
        CHECK (goal_is_active IN (0, 1)),

    CONSTRAINT ck_goals_cadence_enum
        CHECK (cadence IN ('daily','weekly','monthly','mtd','quarterly','yearly')),

    CONSTRAINT ck_goals_default_cadence_enum
        CHECK (default_cadence IS NULL OR default_cadence IN ('daily','weekly','monthly','mtd','quarterly','yearly')),

    CONSTRAINT ck_goals_week_start_enum
        CHECK (week_start IN ('mon','tue','wed','thu','fri','sat','sun')),

    CONSTRAINT ck_goals_tz_len
        CHECK (length(trim(tz)) BETWEEN 1 AND 64),

    -- If dates are provided, basic shape checks + ordering
    CONSTRAINT ck_goals_start_date_format
        CHECK (goal_start_date IS NULL OR goal_start_date GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),

    CONSTRAINT ck_goals_end_date_format
        CHECK (goal_end_date IS NULL OR goal_end_date GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),

    CONSTRAINT ck_goals_date_order
        CHECK (goal_start_date IS NULL OR goal_end_date IS NULL OR goal_end_date >= goal_start_date),

    CONSTRAINT ck_goals_time_order
        CHECK (created_at <= updated_at),

    -- Foreign key
    CONSTRAINT fk_goals_user
        FOREIGN KEY (user_id) REFERENCES users(user_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE
);

-- Uniqueness per user
CREATE UNIQUE INDEX IF NOT EXISTS uq_goals_user_name
    ON goals (user_id, goal_name);

-- Helpful for listing a user's goals quickly
CREATE INDEX IF NOT EXISTS idx_goals_user
    ON goals (user_id);

