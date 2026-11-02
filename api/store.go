package main

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors returned by Store implementations. Handlers translate them
// into HTTP statuses (404 and 409), so stores must return them unwrapped or
// wrapped with %w.
var (
	// ErrNotFound means no product exists with the requested id.
	ErrNotFound = errors.New("not found")
	// ErrInsufficientStock means a stock change would take stock below zero.
	ErrInsufficientStock = errors.New("insufficient stock")
)

// Product is one row of the products table, shaped for JSON output.
//
// shortcut: price is float64, fine for display; switch to a decimal type before doing arithmetic on money.
type Product struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	Stock     int     `json:"stock"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// Store is the persistence layer the HTTP handlers depend on. Handlers only
// see this interface, so tests can swap in a fake instead of a database.
type Store interface {
	// ListProducts returns all products ordered by id. It never returns a nil
	// slice on success, so it encodes as [] rather than null.
	ListProducts(ctx context.Context) ([]Product, error)
	// GetProduct returns the product with the given id, or ErrNotFound.
	GetProduct(ctx context.Context, id int64) (Product, error)
	// CreateProduct inserts a product with zero stock and returns it.
	CreateProduct(ctx context.Context, name string, price float64) (Product, error)
	// AdjustStock adds change (negative to remove) to a product's stock and
	// records the movement with reason, atomically. It returns ErrNotFound for
	// an unknown id and ErrInsufficientStock if stock would go negative.
	AdjustStock(ctx context.Context, id int64, change int, reason string) (Product, error)
	// DeleteProduct removes a product and, by cascade, its stock movements.
	// It returns ErrNotFound for an unknown id.
	DeleteProduct(ctx context.Context, id int64) error
}

// PgStore is the PostgreSQL implementation of Store, backed by a pgx pool.
type PgStore struct{ db *pgxpool.Pool }

// cols is the shared SELECT/RETURNING column list, in the order scan expects.
// Numerics and timestamps are cast so they scan into float64 and string.
const cols = `id, name, price::float8, stock, created_at::text, updated_at::text`

// scan reads one product row (columns as in cols) from r. A missing row is
// reported as ErrNotFound.
func scan(r pgx.Row) (Product, error) {
	var p Product
	err := r.Scan(&p.ID, &p.Name, &p.Price, &p.Stock, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// ListProducts implements Store.
func (s *PgStore) ListProducts(ctx context.Context) ([]Product, error) {
	rows, err := s.db.Query(ctx, `SELECT `+cols+` FROM products ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Product{}
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProduct implements Store.
func (s *PgStore) GetProduct(ctx context.Context, id int64) (Product, error) {
	return scan(s.db.QueryRow(ctx, `SELECT `+cols+` FROM products WHERE id = $1`, id))
}

// CreateProduct implements Store. Stock starts at the column default (0).
func (s *PgStore) CreateProduct(ctx context.Context, name string, price float64) (Product, error) {
	return scan(s.db.QueryRow(ctx,
		`INSERT INTO products (name, price) VALUES ($1, $2) RETURNING `+cols, name, price))
}

// DeleteProduct implements Store.
func (s *PgStore) DeleteProduct(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM products WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AdjustStock implements Store. It updates stock and records the movement in
// one transaction, so the ledger and the stock level cannot drift apart.
// The products.stock CHECK (>= 0) rejects overdrafts, even under concurrent
// updates; that check_violation (SQLSTATE 23514) becomes ErrInsufficientStock.
// Any other failure rolls the transaction back.
func (s *PgStore) AdjustStock(ctx context.Context, id int64, change int, reason string) (Product, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Product{}, err
	}
	defer tx.Rollback(ctx)

	p, err := scan(tx.QueryRow(ctx,
		`UPDATE products SET stock = stock + $2, updated_at = now() WHERE id = $1 RETURNING `+cols, id, change))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" { // check_violation
		return Product{}, ErrInsufficientStock
	}
	if err != nil {
		return Product{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO stock_movements (product_id, change, reason) VALUES ($1, $2, $3)`, id, change, reason); err != nil {
		return Product{}, err
	}
	return p, tx.Commit(ctx)
}
