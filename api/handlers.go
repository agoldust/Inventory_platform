package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// API holds the HTTP handlers and the Store they read and write through.
type API struct{ store Store }

// routes registers every endpoint (including GET /metrics) on a new mux and
// returns it, wrapped in the metrics middleware, as the server's root handler.
// Method and path patterns use Go 1.22 routing.
func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /products", a.list)
	mux.HandleFunc("GET /products/{id}", a.get)
	mux.HandleFunc("POST /products", a.create)
	mux.HandleFunc("PATCH /products/{id}/stock", a.adjustStock)
	mux.HandleFunc("DELETE /products/{id}", a.remove)
	mux.HandleFunc("GET /variant", a.variant)
	mux.Handle("GET /metrics", promhttp.Handler())
	return instrument(mux)
}

// list handles GET /products: 200 with a JSON array of all products.
func (a *API) list(w http.ResponseWriter, r *http.Request) {
	ps, err := a.store.ListProducts(r.Context())
	respond(w, ps, http.StatusOK, err)
}

// get handles GET /products/{id}: 200 with the product, 400 for a bad id,
// 404 if it does not exist.
func (a *API) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := a.store.GetProduct(r.Context(), id)
	respond(w, p, http.StatusOK, err)
}

// create handles POST /products with {"name","price"}: 201 with the new
// product, 400 if the body is invalid, the name is blank or the price is
// negative. Stock always starts at 0; use the stock endpoint to add units.
func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string  `json:"name"`
		Price float64 `json:"price"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Name) == "" || in.Price < 0 {
		writeError(w, http.StatusBadRequest, "name is required and price must be >= 0")
		return
	}
	p, err := a.store.CreateProduct(r.Context(), in.Name, in.Price)
	respond(w, p, http.StatusCreated, err)
}

// adjustStock handles PATCH /products/{id}/stock with {"change","reason"}.
// A positive change adds stock, a negative one removes it. It returns 200 with
// the updated product, 400 for a bad id, a zero change or a blank reason,
// 404 for an unknown product, and 409 if stock would go negative. Each
// successful change increments stock_changes_total.
func (a *API) adjustStock(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Change int    `json:"change"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Change == 0 || strings.TrimSpace(in.Reason) == "" {
		writeError(w, http.StatusBadRequest, "change must be non-zero and reason is required")
		return
	}
	p, err := a.store.AdjustStock(r.Context(), id, in.Change, in.Reason)
	if err == nil {
		direction := "increase"
		if in.Change < 0 {
			direction = "decrease"
		}
		stockChanges.WithLabelValues(direction).Inc()
	}
	respond(w, p, http.StatusOK, err)
}

// remove handles DELETE /products/{id}: 204 with no body on success, 400 for
// a bad id, 404 if it does not exist. The product's stock movements are
// deleted with it.
func (a *API) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteProduct(r.Context(), id); err != nil {
		respond(w, nil, 0, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pathID parses the {id} path value as a positive integer. On failure it
// writes a 400 response and returns false, so callers should just return.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// decode reads the JSON request body into v, rejecting bodies over 1 MiB and
// unknown fields. On failure it writes a 400 response and returns false.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// respond writes v with okStatus when err is nil. Otherwise it maps
// ErrNotFound to 404, ErrInsufficientStock to 409, and anything else to a
// generic 500 (the real error is logged, not sent to the client).
func respond(w http.ResponseWriter, v any, okStatus int, err error) {
	switch {
	case err == nil:
		writeJSON(w, okStatus, v)
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "product not found")
	case errors.Is(err, ErrInsufficientStock):
		writeError(w, http.StatusConflict, "insufficient stock")
	default:
		log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// writeJSON sends v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError sends a JSON error body of the form {"error": msg}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
