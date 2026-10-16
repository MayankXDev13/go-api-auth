// Package cors provides a configurable CORS middleware.
package cors

import (
	"net/http"
	"strings"
)

// Middleware allows browser requests from the configured origins. Requests
// from other origins get no CORS headers, so the browser blocks them.
func Middleware(allowed []string) func(http.Handler) http.Handler {
	allow := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		if o = strings.TrimSpace(o); o != "" {
			allow[o] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			_, ok := allow[origin]
			originAllowed := origin != "" && ok

			if originAllowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Add("Vary", "Origin")
			}

			if r.Method == http.MethodOptions {
				if originAllowed {
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					reqHeaders := r.Header.Get("Access-Control-Request-Headers")
					if reqHeaders == "" {
						reqHeaders = "Authorization, Content-Type"
					}
					w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
					w.Header().Set("Access-Control-Max-Age", "600")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
