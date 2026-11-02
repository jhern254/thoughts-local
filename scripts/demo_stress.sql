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

-- Extra live Subjects exercise scrolling and multiselection without changing
-- the four original event/thought assignments.
INSERT INTO subjects(subject_id,user_id,subject_name) VALUES
 (5,'demo','Go Programming'),(6,'demo','Reading'),(7,'demo','Fitness'),
 (8,'demo','Music'),(9,'demo','Cooking'),(10,'demo','Walking'),
 (11,'demo','Writing'),(12,'demo','Learning'),(13,'demo','Sleep'),
 (14,'demo','Personal projects'),(15,'demo','Work'),(16,'demo','Travel'),
 (17,'demo','Photography'),(18,'demo','Gardening'),(19,'demo','Languages'),
 (20,'demo','日本語の練習'),(21,'demo','Community'),(22,'demo','Family'),
 (23,'demo','Friends'),(24,'demo','Planning'),(25,'demo','Reflection'),
 (26,'demo','Software architecture'),(27,'demo','Accessibility'),
 (28,'demo','Research'),(29,'demo','Small experiments'),
 (30,'demo','A deliberately long Subject name for checking narrow layouts'),
 (31,'demo','Misc'),(32,'demo','Unassigned');
INSERT INTO subjects(subject_id,user_id,subject_name,created_at,updated_at,deleted_at)
 SELECT 33,'demo','Retired experiment',at-86400,at,at FROM demo_clock;

-- Desk breaks stay unassigned; the other activities use their matching subjects.
WITH RECURSIVE hours(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM hours WHERE n<719)
INSERT INTO events (event_id, user_id, subject_id, activity_type, started_at, ended_at, created_at, updated_at)
SELECT n+1, 'demo',
    CASE n%6 WHEN 0 THEN 1 WHEN 1 THEN 1 WHEN 2 THEN 2
         WHEN 3 THEN 3 WHEN 4 THEN 4 ELSE NULL END,
    CASE n%6 WHEN 0 THEN 'Read my own source code' WHEN 1 THEN 'Build the thoughts app'
         WHEN 2 THEN 'Test what I confidently broke' WHEN 3 THEN 'Review a design decision'
         WHEN 4 THEN 'Write docs for future me' ELSE 'Defragment my imaginary desk' END,
    at-(721-n)*3600, at-(720-n)*3600-CASE WHEN n%6=5 THEN 600 ELSE 0 END,
    at-(721-n)*3600, at-(720-n)*3600
FROM hours CROSS JOIN demo_clock;
INSERT INTO events (event_id, user_id, subject_id, activity_type, started_at, created_at, updated_at)
SELECT 721, 'demo', 1, 'Make my own timeline feel instant', at-3600, at-3600, at-3600 FROM demo_clock;

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
-- Three distinct daily mindsets reuse the existing thoughts. Close historical
-- periods at local midnight so they do not accumulate in today's Mindset panel.
WITH daily_thoughts AS (
    SELECT thought_id, observed_at,
        row_number() OVER (
            PARTITION BY date(observed_at, 'unixepoch', 'localtime')
            ORDER BY observed_at, thought_id
        ) AS daily_rank
    FROM thoughts WHERE user_id = 'demo'
)
INSERT INTO thought_mindset_periods (thought_id, started_at, ended_at)
SELECT thought_id, observed_at,
    CASE WHEN date(observed_at, 'unixepoch', 'localtime') <> date(at, 'unixepoch', 'localtime')
        THEN unixepoch(observed_at, 'unixepoch', 'localtime', 'start of day', '+1 day', 'utc') END
FROM daily_thoughts CROSS JOIN demo_clock
WHERE daily_rank <= 3;
-- Forty-eight Goals cover every cadence/priority, optional date bounds,
-- active/inactive filters, long names, Unicode, and varied live membership.
WITH RECURSIVE samples(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM samples WHERE n<48)
INSERT INTO goals(goal_id,user_id,goal_name,target_seconds,priority,goal_is_active,
 cadence,goal_start_date,goal_end_date,created_at,updated_at)
SELECT n,'demo',
 CASE n WHEN 1 THEN 'Build Thoughts — review all Subjects'
 WHEN 2 THEN 'Keep ordinary Misc and Unassigned Subjects explicit'
 WHEN 5 THEN 'Practice 日本語 and review a deliberately long Goal name in a narrow window'
 ELSE CASE n%8 WHEN 0 THEN 'Take a useful walk' WHEN 1 THEN 'Build a small feature'
 WHEN 2 THEN 'Read one useful chapter' WHEN 3 THEN 'Write a clear explanation'
 WHEN 4 THEN 'Practice an instrument' WHEN 5 THEN 'Review what I learned'
 WHEN 6 THEN 'Make time for exercise' ELSE 'Plan the next experiment' END || printf(' %02d',n) END,
 1800+(n%12)*1800,
 CASE n%3 WHEN 1 THEN 'high' WHEN 2 THEN 'normal' ELSE 'low' END,
 n%4<>0,
 CASE n%6 WHEN 0 THEN 'daily' WHEN 1 THEN 'weekly' WHEN 2 THEN 'monthly'
 WHEN 3 THEN 'mtd' WHEN 4 THEN 'quarterly' ELSE 'yearly' END,
 CASE WHEN n%4<>0 THEN date(at,'unixepoch','-14 days') END,
 CASE WHEN n%4 IN(1,2) THEN date(at,'unixepoch','+30 days') END,
 at-86400-n*60,at
FROM samples CROSS JOIN demo_clock;

INSERT INTO subject_goals(subject_id,goal_id,user_id)
SELECT s.subject_id,g.goal_id,'demo' FROM goals g JOIN subjects s ON s.user_id=g.user_id
WHERE s.deleted_at IS NULL AND g.goal_id NOT IN(2,47,48)
 AND s.subject_id<=CASE WHEN g.goal_id=1 THEN 32 ELSE
 CASE g.goal_id%6 WHEN 0 THEN 0 WHEN 1 THEN 1 WHEN 2 THEN 2
 WHEN 3 THEN 3 WHEN 4 THEN 5 ELSE 16 END END;
INSERT INTO subject_goals(subject_id,goal_id,user_id) VALUES
 (31,2,'demo'),(32,2,'demo'),(33,47,'demo');
-- Goal 47 retains only a deleted Subject; Goal 48 has no links. Both show Misc.

DROP TABLE demo_clock;
COMMIT;
