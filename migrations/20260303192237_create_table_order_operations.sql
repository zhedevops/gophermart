-- +goose Up
-- +goose StatementBegin
SELECT 'up SQL query';
CREATE TABLE IF NOT EXISTS order_operations (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    operation SMALLINT NOT NULL CHECK (operation IN (0,1)),
    summ NUMERIC(10,2) NOT NULL,
    status SMALLINT NOT NULL CHECK (status IN (0,1,2,3)),
    order_id INT NOT NULL REFERENCES orders(id),
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_order_operations_order_id ON order_operations(order_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'down SQL query';
DROP INDEX IF EXISTS idx_order_operations_order_id;
DROP TABLE IF EXISTS order_operations CASCADE;
-- +goose StatementEnd
