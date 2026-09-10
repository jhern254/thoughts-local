-- A child goal may support multiple broader parents, with multiple levels.
-- Relationships reflect current attribution, not history. Progress stays on its
-- original goal; rollups sum direct and descendant entries once per ancestor.
-- Deduplicate reachable goal IDs, not durations or paths. Full credit can reach
-- several parents, so their totals are not necessarily additive.
-- Soft-deleted/inactive descendants retain links and contributions. Normal goal
-- lists hide deleted goals; rollups still traverse their retained relationships.
CREATE TABLE IF NOT EXISTS goal_parents (
    goal_id        INTEGER NOT NULL, -- child
    parent_goal_id INTEGER NOT NULL,
    user_id        TEXT NOT NULL, -- both endpoints must have this owner

    CONSTRAINT pk_goal_parents PRIMARY KEY (goal_id, parent_goal_id),
    CONSTRAINT ck_goal_parents_not_self CHECK (goal_id <> parent_goal_id),

    CONSTRAINT fk_goal_parents_child
        FOREIGN KEY (goal_id, user_id) REFERENCES goals(goal_id, user_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE,
    CONSTRAINT fk_goal_parents_parent
        FOREIGN KEY (parent_goal_id, user_id) REFERENCES goals(goal_id, user_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_goal_parents_parent_child
    ON goal_parents (parent_goal_id, goal_id);

-- Reject cycles in the resulting graph, including retained deleted goals.
-- UNION visits each ancestor once, terminating even when the new edge cycles.
CREATE TRIGGER IF NOT EXISTS goal_parents_no_cycle_insert
AFTER INSERT ON goal_parents
BEGIN
    SELECT RAISE(ABORT, 'goal hierarchy cycle') WHERE EXISTS (
        WITH RECURSIVE ancestors(goal_id) AS (
            SELECT NEW.parent_goal_id
            UNION
            SELECT p.parent_goal_id FROM goal_parents p
            JOIN ancestors a ON p.goal_id = a.goal_id
        )
        SELECT 1 FROM ancestors WHERE goal_id = NEW.goal_id
    );
END;

CREATE TRIGGER IF NOT EXISTS goal_parents_no_cycle_update
AFTER UPDATE OF goal_id, parent_goal_id ON goal_parents
BEGIN
    SELECT RAISE(ABORT, 'goal hierarchy cycle') WHERE EXISTS (
        WITH RECURSIVE ancestors(goal_id) AS (
            SELECT NEW.parent_goal_id
            UNION
            SELECT p.parent_goal_id FROM goal_parents p
            JOIN ancestors a ON p.goal_id = a.goal_id
        )
        SELECT 1 FROM ancestors WHERE goal_id = NEW.goal_id
    );
END;
