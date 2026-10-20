ALTER TABLE visual ADD COLUMN video_quality TEXT NOT NULL DEFAULT 'efficient' CHECK (video_quality IN ('efficient','high','maximum'));
