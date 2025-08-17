package types

import "time"

type StatsHour struct {
	Hour  time.Time `json:"hour"`
	Count uint64    `json:"count"`
}

type StatsDay struct {
	Day   time.Time `json:"day"`
	Count uint64    `json:"count"`
}

type Stats24h struct {
	AsOf  time.Time `json:"asof"`
	Count uint64    `json:"count"`
}
