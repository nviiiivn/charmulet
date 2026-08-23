// Package provider abstracts LLM backends behind one interface so core never
// knows which vendor is talking. ollama + first hosted provider arrive in P1.
package provider

import "context"

type Stream func() (chunk string, ok bool, err error)

type Provider interface {
	Name() string
	Models() []string
	// Complete streams a completion for the given prompt. Cancel closes ctx.
	Complete(ctx context.Context, model, prompt string) (Stream, error)
}
