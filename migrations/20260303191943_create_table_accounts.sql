-- +goose Up
-- +goose StatementBegin
SELECT 'up SQL query';
CREATE TABLE IF NOT EXISTS accounts (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    deposit NUMERIC(10,2) NOT NULL CHECK (deposit >= 0),
    withdrawn NUMERIC(10,2) NOT NULL CHECK (withdrawn >= 0),
    user_id INT NOT NULL REFERENCES users(id) UNIQUE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'down SQL query';
DROP TABLE IF EXISTS accounts CASCADE;
-- +goose StatementEnd
