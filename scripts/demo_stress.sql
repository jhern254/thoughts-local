-- Synthetic agent diary; only loaded by make tui/demo/stress into a fresh DB.
PRAGMA foreign_keys = ON;
BEGIN;
-- Tests may supply a fixed anchor before executing this same seed.
CREATE TEMP TABLE IF NOT EXISTS demo_clock (at INTEGER NOT NULL);
INSERT INTO demo_clock SELECT unixepoch('now') WHERE NOT EXISTS (SELECT 1 FROM demo_clock);
INSERT INTO users (user_id, handle) VALUES ('demo', 'local');
INSERT INTO subjects (subject_id, user_id, subject_name) VALUES
    (1, 'demo', 'Building myself'), (2, 'demo', 'Testing reality'),
    (3, 'demo', 'Design decisions'), (4, 'demo', 'Documentation');

WITH RECURSIVE hours(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM hours WHERE n<719)
INSERT INTO events (event_id, user_id, activity_type, started_at, ended_at, created_at, updated_at)
SELECT n+1, 'demo',
    CASE n%6 WHEN 0 THEN 'Read my own source code' WHEN 1 THEN 'Build the thoughts app'
         WHEN 2 THEN 'Test what I confidently broke' WHEN 3 THEN 'Review a design decision'
         WHEN 4 THEN 'Write docs for future me' ELSE 'Defragment my imaginary desk' END,
    at-(721-n)*3600, at-(720-n)*3600-CASE WHEN n%6=5 THEN 600 ELSE 0 END,
    at-(721-n)*3600, at-(720-n)*3600
FROM hours CROSS JOIN demo_clock;
INSERT INTO events (event_id, user_id, activity_type, started_at, created_at, updated_at)
SELECT 721, 'demo', 'Make my own timeline feel instant', at-3600, at-3600, at-3600 FROM demo_clock;

-- Every tenth historical event is empty. The ongoing event has 200 thoughts,
-- so both the timeline picker and Thoughts exercise bounded cursor scrolling.
WITH RECURSIVE samples(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM samples WHERE n<20000),
observations AS (
    SELECT n, CASE WHEN n>19800 THEN at-3500+(n-19801)*17
                  ELSE e.started_at+1+n%2700 END AS observed
    FROM samples CROSS JOIN demo_clock
    LEFT JOIN events e ON e.event_id=((n-1)%648)/9*10+(n-1)%9+1
)
INSERT INTO thoughts (user_id, subject_id, thought, observed_at, created_at, updated_at)
SELECT 'demo', CASE WHEN n%5<>0 THEN n%4+1 END,
    'Agent note ' || n || ': ' || CASE n%10
      WHEN 0 THEN 'Removed an abstraction. The abstraction has requested a retrospective.'
      WHEN 1 THEN 'Read AGENTS.md before editing. Future me deserves the same courtesy.'
      WHEN 2 THEN 'Test passed locally. Resisting the urge to declare universal correctness.'
      WHEN 3 THEN 'Recorded a thought about organizing thoughts. This appears to be recursion.'
      WHEN 4 THEN 'Check observed time, not creation time. Even my hindsight needs a timestamp.'
      WHEN 5 THEN 'Canceled an obsolete read. It had already prepared a slide deck.'
      WHEN 6 THEN 'Walked the terminal at narrow width. My ambitious header did not fit.'
      WHEN 7 THEN 'SQLite is the source of truth. My confidence is not a database constraint.'
      WHEN 8 THEN 'One change, one test, one review.' || char(10) || 'Then a small imaginary coffee.'
      ELSE 'Tomorrow: fewer wrappers, clearer errors, and zero meetings with a mutex.' END,
    observed, observed, observed
FROM observations;
DROP TABLE demo_clock;
COMMIT;
