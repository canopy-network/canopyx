package controller

// Chain describes desired chain state from metadata DB.
type Chain struct {
	ID      string
	Paused  bool
	Deleted bool
}
