-- Drop thoughts table.
DROP INDEX IF EXISTS idx_thoughts_user_observed_at;
DROP INDEX IF EXISTS idx_thoughts_subject;
DROP INDEX IF EXISTS idx_thoughts_event;
DROP INDEX IF EXISTS idx_thoughts_active_user_observed_created_id;
DROP TABLE IF EXISTS thoughts;
