package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeStore is an in-memory Store stub. Every method returns a fixed product
// and the configured err, so a test picks the outcome by setting err.
type fakeStore struct{ err error }

// ListProducts returns an empty list and f.err.
func (f fakeStore) ListProducts(context.Context) ([]Product, error) { return []Product{}, f.err }

// GetProduct returns product 1 and f.err.
func (f fakeStore) GetProduct(context.Context, int64) (Product, error) {
	return Product{ID: 1}, f.err
}

// CreateProduct echoes the name and price back as product 1, with f.err.
func (f fakeStore) CreateProduct(_ context.Context, name string, price float64) (Product, error) {
	return Product{ID: 1, Name: name, Price: price}, f.err
}

// AdjustStock returns product 1 and f.err, ignoring the change.
func (f fakeStore) AdjustStock(context.Context, int64, int, string) (Product, error) {
	return Product{ID: 1}, f.err
}

// TestAPI is a table-driven test of every route. It checks the HTTP status
// for the success path, input validation, and store errors (404 and 409).
func TestAPI(t *testing.T) {
	tests := []struct {
		name   string
		store  fakeStore
		method string
		path   string
		body   string
		want   int
	}{
		{"healthz", fakeStore{}, "GET", "/healthz", "", 200},
		{"list", fakeStore{}, "GET", "/products", "", 200},
		{"get ok", fakeStore{}, "GET", "/products/1", "", 200},
		{"get bad id", fakeStore{}, "GET", "/products/abc", "", 400},
		{"get missing", fakeStore{ErrNotFound}, "GET", "/products/9", "", 404},
		{"create ok", fakeStore{}, "POST", "/products", `{"name":"Pen","price":1.5}`, 201},
		{"create negative price", fakeStore{}, "POST", "/products", `{"name":"Pen","price":-1}`, 400},
		{"create bad json", fakeStore{}, "POST", "/products", `{`, 400},
		{"stock ok", fakeStore{}, "PATCH", "/products/1/stock", `{"change":-2,"reason":"sale"}`, 200},
		{"stock zero change", fakeStore{}, "PATCH", "/products/1/stock", `{"change":0,"reason":"x"}`, 400},
		{"stock no reason", fakeStore{}, "PATCH", "/products/1/stock", `{"change":1}`, 400},
		{"stock overdraft", fakeStore{ErrInsufficientStock}, "PATCH", "/products/1/stock", `{"change":-99,"reason":"sale"}`, 409},
		{"stock missing product", fakeStore{ErrNotFound}, "PATCH", "/products/9/stock", `{"change":1,"reason":"restock"}`, 404},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			(&API{store: tc.store}).routes().ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d: %s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}
