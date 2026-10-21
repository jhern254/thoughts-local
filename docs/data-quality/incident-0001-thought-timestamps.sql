-- Incident 1: read-only review of potentially affected thoughts.
-- NULL/unbound cutoff returns every row. For a bounded extract, validate the
-- cutoff and creation timestamps as described in the incident report first.
SELECT
    t.*,
    datetime(t.observed_at, 'unixepoch') AS observed_at_utc,
    datetime(t.created_at, 'unixepoch') AS created_at_utc,
    datetime(t.updated_at, 'unixepoch') AS updated_at_utc,
    datetime(t.deleted_at, 'unixepoch') AS deleted_at_utc
FROM thoughts AS t
WHERE :fixed_writers_started_at_unix IS NULL
   OR t.created_at <= :fixed_writers_started_at_unix
ORDER BY t.created_at, t.thought_id;
