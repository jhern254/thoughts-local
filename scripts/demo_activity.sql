-- Varied two-year history for day-activity review; the original stress seed
-- remains the stable fixture for existing performance comparisons.
PRAGMA foreign_keys = ON;
BEGIN;
CREATE TEMP TABLE IF NOT EXISTS demo_clock (at INTEGER NOT NULL);
INSERT INTO demo_clock SELECT unixepoch('now') WHERE NOT EXISTS (SELECT 1 FROM demo_clock);
INSERT INTO users (user_id, handle) VALUES ('demo', 'local');
INSERT INTO subjects (subject_id, user_id, subject_name) VALUES
    (1, 'demo', 'Learning'), (2, 'demo', 'Building'),
    (3, 'demo', 'Wellbeing'), (4, 'demo', 'Reflection');

-- Six repeatable seasons give independent channels and genuinely empty gaps.
-- Historical slots finish at least a day before launch, regardless of timezone.
CREATE TEMP TABLE activity_days AS
WITH RECURSIVE days(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM days WHERE n<729)
SELECT n, at-n*86400-43200 AS started,
    CASE WHEN n<=13 THEN (n*7)%11
         WHEN n%120 BETWEEN 14 AND 27 THEN 8+n%7
         WHEN n%120 BETWEEN 28 AND 42 THEN 1+n%2
         WHEN n%120 BETWEEN 43 AND 55 THEN 1+n%3
         WHEN n%120 BETWEEN 56 AND 80 THEN 3+n%6
         WHEN n%120 BETWEEN 105 AND 119 THEN 18+n%7
         ELSE 0 END AS event_count,
    CASE WHEN n<=13 THEN (n*37)%181
         WHEN n%120 BETWEEN 14 AND 27 THEN 3+n%9
         WHEN n%120 BETWEEN 28 AND 42 THEN 60+(n%10)*16
         WHEN n%120 BETWEEN 43 AND 55 THEN n%4
         WHEN n%120 BETWEEN 56 AND 80 THEN 12+n%35
         WHEN n%120 BETWEEN 105 AND 119 THEN 80+(n%5)*30
         ELSE 0 END AS thought_count
FROM days CROSS JOIN demo_clock;

WITH RECURSIVE slots(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM slots WHERE n<23)
INSERT INTO events (user_id, subject_id, activity_type, started_at, ended_at, created_at, updated_at)
SELECT 'demo', slots.n%4+1,
    CASE slots.n%4 WHEN 0 THEN 'Study and experiment' WHEN 1 THEN 'Build a small project'
         WHEN 2 THEN 'Walk and recharge' ELSE 'Review the day' END,
    started+slots.n*1800, started+slots.n*1800+900+(slots.n%3)*300,
    started+slots.n*1800, started+slots.n*1800+900+(slots.n%3)*300
FROM activity_days JOIN slots ON slots.n<event_count;

WITH RECURSIVE samples(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM samples WHERE n<204)
INSERT INTO thoughts (user_id, subject_id, thought, observed_at, created_at, updated_at)
SELECT 'demo', CASE WHEN samples.n%5<>0 THEN samples.n%4+1 END,
    'Day ' || activity_days.n || ', note ' || samples.n || ': ' ||
    CASE samples.n%4 WHEN 0 THEN 'Keep the next step small.'
         WHEN 1 THEN 'Tried a different approach and wrote down what changed.'
         WHEN 2 THEN 'A quiet day can still contain something worth remembering.'
         ELSE 'Review the experiment, its assumptions, and the result before deciding what to do next.' END,
    started+(samples.n*337)%43200, started+(samples.n*337)%43200, started+(samples.n*337)%43200
FROM activity_days JOIN samples ON samples.n<=thought_count;

-- A few boundary cases, separate from the ordinary daily slots.
INSERT INTO events (user_id, activity_type, started_at, ended_at)
SELECT 'demo', 'Overnight trip', started+36000, started+129600 FROM activity_days WHERE n IN (37,365);
INSERT INTO events (user_id, activity_type, started_at, ended_at)
SELECT 'demo', 'A brief check-in', started+40000, started+40000 FROM activity_days WHERE n=55;

-- Always open on usable content: two completed cards and one ongoing card with
-- 200 Thoughts for expansion, paging and return-navigation checks.
INSERT INTO events (user_id, subject_id, activity_type, started_at, ended_at)
SELECT 'demo', 1, 'Inspect the day curves', at-10800, at-7200 FROM demo_clock
UNION ALL SELECT 'demo', 2, 'Try zoom and series toggles', at-7200, at-3600 FROM demo_clock;
INSERT INTO events (user_id, subject_id, activity_type, started_at)
SELECT 'demo', 4, 'Explore the activity demo', at-3600 FROM demo_clock;
WITH RECURSIVE samples(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM samples WHERE n<200)
INSERT INTO thoughts (user_id, subject_id, thought, observed_at)
SELECT 'demo', CASE WHEN n%5<>0 THEN 4 END,
    'Current note ' || n || ': ' || CASE n%3
      WHEN 0 THEN 'Press Home then Up to explore day activity.'
      WHEN 1 THEN 'Zoom into a busy period, then move one day at a time.'
      ELSE 'Compare Events and Thoughts, then expand a card and browse its notes.' END,
    at-3500+(n-1)*17
FROM samples CROSS JOIN demo_clock;
DROP TABLE activity_days;
DROP TABLE demo_clock;
COMMIT;
