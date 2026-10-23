CREATE TABLE visual (
    user_id TEXT PRIMARY KEY NOT NULL REFERENCES users(user_id),
    background_asset TEXT,
    darkness INTEGER NOT NULL DEFAULT 70 CHECK (typeof(darkness) = 'integer' AND darkness BETWEEN 0 AND 95),
    fit TEXT NOT NULL DEFAULT 'fill' CHECK (fit IN ('fill', 'fit')),
    zoom INTEGER NOT NULL DEFAULT 100 CHECK (typeof(zoom) = 'integer' AND zoom BETWEEN 100 AND 300),
    position_x INTEGER NOT NULL DEFAULT 5000 CHECK (typeof(position_x) = 'integer' AND position_x BETWEEN 0 AND 10000),
    position_y INTEGER NOT NULL DEFAULT 5000 CHECK (typeof(position_y) = 'integer' AND position_y BETWEEN 0 AND 10000),
    speed_percent INTEGER NOT NULL DEFAULT 100 CHECK (typeof(speed_percent) = 'integer' AND speed_percent BETWEEN 25 AND 100),
    user_paused INTEGER NOT NULL DEFAULT 0 CHECK (typeof(user_paused) = 'integer' AND user_paused IN (0, 1)),
    video_quality TEXT NOT NULL DEFAULT 'efficient' CHECK (video_quality IN ('efficient', 'high', 'maximum')),
    thicker_card_borders INTEGER NOT NULL DEFAULT 0 CHECK (typeof(thicker_card_borders) = 'integer' AND thicker_card_borders IN (0, 1)),
    heavier_text INTEGER NOT NULL DEFAULT 0 CHECK (typeof(heavier_text) = 'integer' AND heavier_text IN (0, 1)),
    CHECK (background_asset IS NULL OR (
        length(background_asset) = 36
        AND substr(background_asset, 1, 32) NOT GLOB '*[^0-9a-f]*'
        AND substr(background_asset, 33) IN ('.jpg', '.png', '.mp4')
    ))
);
