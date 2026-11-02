CREATE TABLE products (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT           NOT NULL,
    price      NUMERIC(10,2)  NOT NULL CHECK (price >= 0),
    stock      INTEGER        NOT NULL DEFAULT 0 CHECK (stock >= 0),
    created_at TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ    NOT NULL DEFAULT now()
);
