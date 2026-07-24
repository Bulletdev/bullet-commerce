package middleware

import "net/http"

// SecurityHeaders hardens every response with defense-in-depth browser headers.
// Even for a JSON API these are cheap: nosniff stops content-type sniffing,
// DENY plus frame-ancestors block clickjacking, HSTS pins HTTPS once a client
// has seen it, the null CSP and no-referrer keep any accidental HTML inert, and
// X-XSS-Protection: 0 disables the legacy (and exploitable) browser XSS auditor
// as OWASP recommends.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		h.Set("X-XSS-Protection", "0")
		next.ServeHTTP(w, r)
	})
}
