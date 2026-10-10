ALTER TABLE visual ADD COLUMN fit TEXT NOT NULL DEFAULT 'fill' CHECK (fit IN ('fill', 'fit'));
ALTER TABLE visual ADD COLUMN zoom INTEGER NOT NULL DEFAULT 100 CHECK (typeof(zoom) = 'integer' AND zoom BETWEEN 100 AND 300);
ALTER TABLE visual ADD COLUMN position_x INTEGER NOT NULL DEFAULT 5000 CHECK (typeof(position_x) = 'integer' AND position_x BETWEEN 0 AND 10000);
ALTER TABLE visual ADD COLUMN position_y INTEGER NOT NULL DEFAULT 5000 CHECK (typeof(position_y) = 'integer' AND position_y BETWEEN 0 AND 10000);
