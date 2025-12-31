package controller

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4/json"
	"github.com/golang-jwt/jwt/v5"
)

// ValidateToken checks if the Authorization header contains a valid AdminToken
func (c *Controller) ValidateToken(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		return token == c.AdminToken
	}
	return false
}

// ValidateSessionCookie checks if the session cookie is present and valid
func (c *Controller) ValidateSessionCookie(r *http.Request) bool {
	cookie, err := r.Cookie("cx_session")
	if err != nil {
		return false
	}
	tok, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (any, error) { return c.JWTSecret, nil })
	return err == nil && tok.Valid
}

// ValidateRole checks the role in a valid session cookie
func (c *Controller) ValidateRole(r *http.Request, role string) bool {
	cookie, err := r.Cookie("cx_session")
	if err != nil {
		return false
	}

	tok, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (any, error) { return c.JWTSecret, nil })
	if err != nil || !tok.Valid {
		return false
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return false
	}

	tokenRole, _ := claims["role"].(string)
	return tokenRole == role
}

// RequireAuth middleware
func (c *Controller) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.ValidateToken(r) || c.ValidateSessionCookie(r) { // X-Session used by CSR fetches
			next.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	})
}

// RequireAdmin middleware
func (c *Controller) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.ValidateToken(r) || (c.ValidateSessionCookie(r) && c.ValidateRole(r, "admin")) {
			next.ServeHTTP(w, r)
			return
		}

		// Unauthorized
		if !c.ValidateSessionCookie(r) {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		// Forbidden
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden"})
	})
}

// IssueSession issues a session cookie
func (c *Controller) IssueSession(w http.ResponseWriter, username string) {
	ttl := 8 * time.Hour
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  username,
		"role": "admin",
		"exp":  time.Now().Add(ttl).Unix(),
		"iat":  time.Now().Unix(),
	})
	ss, _ := token.SignedString(c.JWTSecret)
	http.SetCookie(w, &http.Cookie{
		Name:     "cx_session",
		Value:    ss,
		Path:     "/",
		HttpOnly: true,
		Secure:   os.Getenv("ENVIRONMENT") == "production",
		SameSite: http.SameSiteStrictMode,
		// Persist across admin restarts:
		MaxAge: int(ttl.Seconds()),
	})
}

// currentUser returns the username associated with the request when available.
// API tokens are treated as admin-equivalent and return "api-token".
func (c *Controller) currentUser(r *http.Request) string {
	// Check API token first (admin-equivalent)
	if c.ValidateToken(r) {
		return "api-token"
	}
	// Check session cookie
	if cookie, err := r.Cookie("cx_session"); err == nil {
		if tok, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (any, error) { return c.JWTSecret, nil }); err == nil && tok.Valid {
			if claims, ok := tok.Claims.(jwt.MapClaims); ok {
				if sub, _ := claims["sub"].(string); sub != "" {
					return sub
				}
			}
		}
	}
	return "unknown"
}
