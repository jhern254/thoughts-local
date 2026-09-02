-- Drop subjects table.
DROP INDEX IF EXISTS idx_subjects_user;
DROP INDEX IF EXISTS uq_subjects_id_user;
DROP INDEX IF EXISTS uq_subjects_user_name;
DROP TABLE IF EXISTS subjects;
