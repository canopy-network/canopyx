package controller

import (
	"net/http"
	"time"

	"github.com/go-jose/go-jose/v4/json"
	"golang.org/x/crypto/bcrypt"
)

// HandleAdminLogin handles admin login
func (c *Controller) HandleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad json"})
		return
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	u, ok := c.Users[in.Username]
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
		return
	}
	if err := bcrypt.CompareHashAndPassword(u.Hash, []byte(in.Password)); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
		return
	}
	c.IssueSession(w, in.Username)
	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
}

// HandleAdminLogout handles admin logout
func (c *Controller) HandleAdminLogout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "cx_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	w.WriteHeader(http.StatusNoContent)
}
