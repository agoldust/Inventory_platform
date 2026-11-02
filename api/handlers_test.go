package main

import (
	"context"
	"net/http"
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

// TestMetrics makes a few requests and checks that /metrics reports the
// per-route counter, the latency histogram, and the stock change counter.
func TestMetrics(t *testing.T) {
	h := (&API{store: fakeStore{}}).routes()
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	do("GET", "/products/1", "")
	do("PATCH", "/products/1/stock", `{"change":2,"reason":"restock"}`)
	do("PATCH", "/products/1/stock", `{"change":-1,"reason":"sale"}`)
	do("GET", "/nope", "")
	req := httptest.NewRequest("GET", "/products", nil)
	req.AddCookie(&http.Cookie{Name: variantCookie, Value: "B"})
	h.ServeHTTP(httptest.NewRecorder(), req)

	out := do("GET", "/metrics", "").Body.String()
	for _, want := range []string{
		`http_requests_total{route="GET /products/{id}",status="200",variant="none"}`,
		`http_request_duration_seconds_bucket{route="GET /products/{id}",variant="none",le="0.005"}`,
		`http_requests_total{route="unmatched",status="404",variant="none"}`,
		`http_requests_total{route="GET /products",status="200",variant="B"}`,
		`stock_changes_total{direction="increase"}`,
		`stock_changes_total{direction="decrease"}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("/metrics is missing %s", want)
		}
	}
}

// TestVariant checks cookie assignment: a new visitor gets A or B plus a
// cookie, a returning visitor keeps theirs, and a tampered cookie is replaced.
func TestVariant(t *testing.T) {
	h := (&API{store: fakeStore{}}).routes()
	get := func(cookie string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/variant", nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: variantCookie, Value: cookie})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		rec := get("")
		cs := rec.Result().Cookies()
		if len(cs) != 1 || (cs[0].Value != "A" && cs[0].Value != "B") {
			t.Fatalf("new visitor: want one A/B cookie, got %v", cs)
		}
		if !strings.Contains(rec.Body.String(), `"variant":"`+cs[0].Value+`"`) {
			t.Fatalf("body %q does not match cookie %s", rec.Body, cs[0].Value)
		}
		seen[cs[0].Value] = true
	}
	if !seen["A"] || !seen["B"] {
		t.Errorf("100 new visitors should hit both variants, got %v", seen)
	}

	rec := get("B")
	if len(rec.Result().Cookies()) != 0 || !strings.Contains(rec.Body.String(), `"variant":"B"`) {
		t.Errorf("returning visitor should keep B without a new cookie, got %d cookies, body %q", len(rec.Result().Cookies()), rec.Body)
	}

	if cs := get("evil").Result().Cookies(); len(cs) != 1 || (cs[0].Value != "A" && cs[0].Value != "B") {
		t.Errorf("tampered cookie should be replaced with A or B, got %v", cs)
	}
}

// DeleteProduct returns f.err.
func (f fakeStore) DeleteProduct(context.Context, int64) error { return f.err }

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
		{"delete ok", fakeStore{}, "DELETE", "/products/1", "", 204},
		{"delete bad id", fakeStore{}, "DELETE", "/products/abc", "", 400},
		{"delete missing", fakeStore{ErrNotFound}, "DELETE", "/products/9", "", 404},
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
