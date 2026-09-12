-- Only loaded into the disposable database by make tui/demo/seeded.
PRAGMA foreign_keys = ON;
BEGIN;
INSERT INTO users (user_id, handle) VALUES ('demo', 'local');
INSERT INTO subjects (subject_id, user_id, subject_name) VALUES
    (1, 'demo', 'Reading'), (2, 'demo', 'Building');
INSERT INTO events (user_id, activity_type, started_at, ended_at) VALUES
    ('demo', 'Reading', unixepoch('now') - 10800, unixepoch('now') - 7200),
    ('demo', 'Taking a walk', unixepoch('now') - 7200, unixepoch('now') - 3600),
    ('demo', 'Building something small', unixepoch('now') - 3600, NULL);

-- Relative times keep the demo useful on every launch. Near midnight, older
-- events appear on the previous day. The ongoing event has eight previews.
WITH RECURSIVE samples(n) AS (
    VALUES(1) UNION ALL SELECT n + 1 FROM samples WHERE n < 12
)
INSERT INTO thoughts (user_id, subject_id, thought, observed_at)
SELECT 'demo',
    CASE WHEN n <= 2 THEN 1 WHEN n > 4 THEN 2 END,
    CASE WHEN n <= 2 THEN 'A useful idea from today''s reading: explain it in my own words.'
         WHEN n <= 4 THEN 'A walk leaves room to notice something new.'
         ELSE 'Small experiment ' || (n - 4) || ': make one change, try it, and write down what happened.' END,
    CASE WHEN n <= 4 THEN unixepoch('now') - 10800 + n * 1500
         ELSE unixepoch('now') - 3600 + (n - 4) * 400 END
FROM samples;
COMMIT;
