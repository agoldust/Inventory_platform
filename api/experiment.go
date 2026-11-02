package main

import (
	"math/rand/v2"
	"net/http"
)

// variantCookie holds a visitor's A/B test variant ("A" or "B").
const variantCookie = "ab_variant"

// variantOf returns the variant from the request's cookie: "A" or "B", or
// "none" if the cookie is missing or holds anything else. Accepting only
// these values keeps the Prometheus variant label from growing without bound.
func variantOf(r *http.Request) string {
	if c, err := r.Cookie(variantCookie); err == nil && (c.Value == "A" || c.Value == "B") {
		return c.Value
	}
	return "none"
}

// variant handles GET /variant: it returns the caller's variant as
// {"variant": "A"|"B"}. A caller without a valid cookie gets one assigned at
// random (50/50) and kept for a year, so the same browser always sees the
// same layout. This is the only place a variant is assigned; other clients,
// such as Prometheus, stay "none".
func (a *API) variant(w http.ResponseWriter, r *http.Request) {
	v := variantOf(r)
	if v == "none" {
		v = [2]string{"A", "B"}[rand.IntN(2)]
		http.SetCookie(w, &http.Cookie{
			Name:     variantCookie,
			Value:    v,
			Path:     "/",
			MaxAge:   365 * 24 * 3600,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}
	writeJSON(w, http.StatusOK, map[string]string{"variant": v})
}
