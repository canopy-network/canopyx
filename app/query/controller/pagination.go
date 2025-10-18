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

// SortOrder represents the sort direction for queries
type SortOrder string

const (
	SortOrderAsc  SortOrder = "asc"
	SortOrderDesc SortOrder = "desc"
)

type pageSpec struct {
	Limit  int
	Cursor uint64
	Sort   SortOrder
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

	// Parse sort parameter, default to "desc" (newest first)
	sort := SortOrderDesc
	if v := qs.Get("sort"); v != "" {
		switch v {
		case "asc":
			sort = SortOrderAsc
		case "desc":
			sort = SortOrderDesc
		default:
			return pageSpec{}, errInvalidSort
		}
	}

	return pageSpec{Limit: limit, Cursor: cursor, Sort: sort}, nil
}

var (
	errInvalidLimit  = &parseError{msg: "invalid limit"}
	errInvalidCursor = &parseError{msg: "invalid cursor"}
	errInvalidSort   = &parseError{msg: "invalid sort, must be 'asc' or 'desc'"}
)

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }
