-- +goose Up
CREATE TABLE IF NOT EXISTS image_items (
    id UUID PRIMARY KEY,
    object_id UUID NOT NULL,
    ext TEXT NOT NULL,
    gender TEXT,
    category TEXT,
    style TEXT,
    color TEXT,
    season TEXT,
    material TEXT,
    description TEXT
);

-- +goose Down
DROP TABLE IF EXISTS image_items;
