CREATE TABLE visual (
    user_id TEXT PRIMARY KEY NOT NULL REFERENCES users(user_id),
    background_asset TEXT,
    darkness INTEGER NOT NULL DEFAULT 70 CHECK (typeof(darkness) = 'integer' AND darkness BETWEEN 0 AND 95),
    CHECK (background_asset IS NULL OR (
        length(background_asset) = 36
        AND substr(background_asset, 1, 32) NOT GLOB '*[^0-9a-f]*'
        AND substr(background_asset, 33) IN ('.jpg', '.png')
    ))
);
