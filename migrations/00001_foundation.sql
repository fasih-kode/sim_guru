-- +goose Up
-- Domain tables will live in this namespace; no product data is seeded here.
CREATE SCHEMA app;

-- +goose Down
-- RESTRICT intentionally refuses rollback when domain objects still exist.
DROP SCHEMA app RESTRICT;
