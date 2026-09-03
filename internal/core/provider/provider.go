// Package provider abstracts LLM backends behind one interface so core never
// knows which vendor is talking.
package provider

import "context"

// Message is one turn of a conversation sent to a backend.
type Message struct {
	Role    string `json:"role"` // "system" | "user" | "assistant"
	Content string `json:"content"`
}

// Stream yields chunks until ok=false (end) or err!=nil.
type Stream func() (chunk string, ok bool, err error)

type Provider interface {
	Name() string
	Models() []string
	// Complete streams a completion for the conversation. Cancel closes ctx.
	Complete(ctx context.Context, model string, msgs []Message) (Stream, error)
}
