package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nvii/charmulet/internal/core/provider"
)

func TestModelsParsesAndCaches(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		fmt.Fprint(w, `{"models":[{"name":"qwen3.5:9b"},{"name":"gemma4:e4b"}]}`)
	}))
	defer srv.Close()

	c := New(srv.URL)
	got := c.Models()
	if len(got) != 2 || got[0] != "qwen3.5:9b" || got[1] != "gemma4:e4b" {
		t.Fatalf("models: %v", got)
	}
	again := c.Models()
	if len(again) != 2 {
		t.Fatalf("cached models: %v", again)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("cache broken: %d upstream hits", hits)
	}
}

func TestCompleteStreamsNDJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"Hello"}}`)
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":""}}`)
		flusher := w.(http.Flusher)
		flusher.Flush()
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":" world"}}`)
		fmt.Fprintln(w, `{"done":true}`)
	}))
	defer srv.Close()

	c := New(srv.URL)
	stream, err := c.Complete(context.Background(), "qwen3.5:9b", []provider.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	var sb string
	for {
		tok, ok, err := stream()
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		if !ok {
			break
		}
		sb += tok
	}
	if sb != "Hello world" {
		t.Fatalf("stream text %q", sb)
	}
	// stream must end cleanly, not error
	if tok, ok, err := stream(); ok || err != nil || tok != "" {
		t.Fatalf("post-done stream not clean: %q %v %v", tok, ok, err)
	}
}

func TestCompleteHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.Complete(context.Background(), "nope:1b", nil)
	if err == nil {
		t.Fatal("404 must error")
	}
}

func TestCompleteCancelEndsCleanly(t *testing.T) {
	released := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"first"}}`)
		w.(http.Flusher).Flush()
		<-released // hold the connection open until the test ends
	}))
	defer srv.Close()
	defer close(released)

	ctx, cancel := context.WithCancel(context.Background())
	c := New(srv.URL)
	stream, err := c.Complete(ctx, "qwen3.5:9b", nil)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if tok, ok, err := stream(); !ok || err != nil || tok != "first" {
		t.Fatalf("first chunk: %q %v %v", tok, ok, err)
	}

	// cancel while the server is mid-stream; the reader must end cleanly
	cancel()
	done := make(chan struct{})
	var finalErr error
	go func() {
		defer close(done)
		_, _, finalErr = stream()
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled stream did not return")
	}
	if finalErr != nil {
		t.Fatalf("cancel must be a clean end, got %v", finalErr)
	}
}
