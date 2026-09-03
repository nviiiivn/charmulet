package chat

import "testing"

func TestSessionTitle(t *testing.T) {
	cases := map[string]string{
		"qwen3.5:9b": "QWEN3.5",
		"hf.co/NousResearch/Hermes-4.3-36B-GGUF:q4_K_M": "HERMES-4.3-36…",
		"gemma4:e4b": "GEMMA4",
	}
	for in, want := range cases {
		if got := sessionTitle(in); got != want {
			t.Errorf("sessionTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
