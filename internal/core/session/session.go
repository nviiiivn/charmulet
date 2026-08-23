// Package session owns conversation persistence. P0 placeholder types; the
// SQLite store arrives in P1.
package session

import "time"

type Session struct {
	ID        string
	Title     string
	CreatedAt time.Time
	Messages  []Message
}

type Message struct {
	Role    string // user | assistant | tool
	Content string
	At      time.Time
}
