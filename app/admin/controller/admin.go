package controller

import (
	"net/http"
	"path"
	"path/filepath"
	"time"

	"github.com/canopy-network/canopyx/pkg/utils"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/gorilla/mux"
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

// LoadAdminUI loads the admin UI
func (c *Controller) LoadAdminUI(r *mux.Router) error {
	defaultAdminWebPath := path.Join("web", "admin", "out")
	uiPath := utils.Env("ADMIN_STATIC_DIR", defaultAdminWebPath)

	// Static admin UI (filesystem-based, optional)
	if uiPath == "" {
		// do nothing if no path provided
		return nil
	}

	absolutePath, err := filepath.Abs(uiPath)
	if err != nil {
		return err
	}

	// Admin API - Login/Logout are only available when the static admin UI is enabled
	r.HandleFunc("/auth/login", c.HandleAdminLogin).Methods(http.MethodPost)
	r.HandleFunc("/auth/logout", c.HandleAdminLogout).Methods(http.MethodPost)

	// TODO: add a way to redirect from /admin to /admin/
	fs := http.FileServer(http.Dir(absolutePath)) // dir should point to the Next.js 'out' folder
	// Next assets live under /admin/_next/*
	r.PathPrefix("/_next/").Handler(fs)
	// App routes (exported files) live under /admin/*
	r.PathPrefix("/").Handler(fs)

	return nil
}
