-- +goose Up
-- citext gives case-insensitive text for e-mail addresses (used from M2).
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose Down
DROP EXTENSION IF EXISTS citext;
