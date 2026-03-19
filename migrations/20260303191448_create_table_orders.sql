-- +goose Up
-- +goose StatementBegin
SELECT 'up SQL query';
CREATE TABLE IF NOT EXISTS orders (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number VARCHAR(15) NOT NULL UNIQUE,
    status SMALLINT NOT NULL CHECK (status IN (0,1,2,3)),
    user_id INT NOT NULL REFERENCES users(id),
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_orders_number ON orders(number);
CREATE INDEX idx_orders_number_user_id ON orders(number, user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'down SQL query';
DROP INDEX IF EXISTS idx_orders_number;
DROP INDEX IF EXISTS idx_orders_number_user_id;
DROP TABLE IF EXISTS orders CASCADE;
-- +goose StatementEnd
