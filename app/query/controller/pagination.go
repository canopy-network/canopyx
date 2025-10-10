package controller

import (
	"math"
	"net/http"
	"strconv"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

type pageSpec struct {
	Limit  int
	Cursor uint64
}

func parsePageSpec(r *http.Request) (pageSpec, error) {
	qs := r.URL.Query()
	limit := defaultLimit
	if v := qs.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err != nil || n <= 0 {
			return pageSpec{}, errInvalidLimit
		} else {
			limit = int(math.Min(float64(n), maxLimit))
		}
	}

	var cursor uint64
	if v := qs.Get("cursor"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return pageSpec{}, errInvalidCursor
		}
		cursor = n
	}

	return pageSpec{Limit: limit, Cursor: cursor}, nil
}

var (
	errInvalidLimit  = &parseError{msg: "invalid limit"}
	errInvalidCursor = &parseError{msg: "invalid cursor"}
)

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }
