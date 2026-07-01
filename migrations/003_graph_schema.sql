-- +goose Up
CREATE SCHEMA IF NOT EXISTS graph;

-- +goose Down
DROP SCHEMA IF EXISTS graph CASCADE;
