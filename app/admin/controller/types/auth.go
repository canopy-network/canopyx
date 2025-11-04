package types

// LoginRequest contains credentials for admin authentication
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
