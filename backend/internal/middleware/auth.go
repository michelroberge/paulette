package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

// SessionClaimsKey is the context key used to store parsed session claims.
const SessionClaimsKey contextKey = "oidcClaims"

// SessionClaims holds the user identity extracted from a validated session JWT.
type SessionClaims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
	jwt.RegisteredClaims
}

// OIDCAuth returns a chi-compatible middleware that validates the paulette_session
// HttpOnly cookie. When enabled is false it returns a no-op pass-through.
// Paths under /api/auth/ are always allowed through without a session check.
func OIDCAuth(enabled bool, sessionSecret string) func(http.Handler) http.Handler {
	if !enabled {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only guard /api/ routes; static files and the SPA must load freely.
			// Within /api/, exempt auth and config endpoints so the frontend can
			// discover OIDC is enabled and reach the login flow.
			if !strings.HasPrefix(r.URL.Path, "/api/") ||
				strings.HasPrefix(r.URL.Path, "/api/auth/") ||
				r.URL.Path == "/api/config" {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie("paulette_session")
			if err != nil {
				writeUnauthorized(w)
				return
			}

			claims := &SessionClaims{}
			token, err := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(sessionSecret), nil
			})
			if err != nil || !token.Valid {
				writeUnauthorized(w)
				return
			}

			ctx := context.WithValue(r.Context(), SessionClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"}) //nolint:errcheck
}
