CREATE TABLE stock_movements (
    id         BIGSERIAL PRIMARY KEY,
    product_id BIGINT      NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    change     INTEGER     NOT NULL CHECK (change <> 0),
    reason     TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_stock_movements_product_id ON stock_movements (product_id);
