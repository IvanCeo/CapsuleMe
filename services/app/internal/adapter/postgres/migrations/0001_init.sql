-- +goose Up

CREATE TABLE feedback (
    id BIGSERIAL PRIMARY KEY,
    score VARCHAR(10) NOT NULL,
    capsule TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS image_items;
