package dsc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newFIMTestClient(apiKey, baseURL string) *Deepseek {
	httpClient = &http.Client{Timeout: 30 * time.Second}
	return &Deepseek{
		apiKey:     apiKey,
		baseURL:    baseURL,
		maxRetries: 3,
		retryDelay: 10 * time.Millisecond,
	}
}

func TestFIMNonStreaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/beta/completions" {
			t.Errorf("expected /beta/completions, got %s", r.URL.Path)
		}

		resp := FIMResponse{
			ID: "cmpl-test-123",
			Choices: []FIMChoice{
				{
					Text:         " world!",
					Index:        0,
					FinishReason: "stop",
				},
			},
			Usage: FIMUsage{
				CompletionTokens: 2,
				PromptTokens:     3,
				TotalTokens:      5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newFIMTestClient("test-key", server.URL)

	resp, err := client.FIM(context.Background(), FIMRequest{
		Model:       "deepseek-v4-pro",
		Prompt:      "Hello,",
		Suffix:      "!",
		MaxTokens:   100,
		Temperature: 0.7,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "cmpl-test-123" {
		t.Errorf("expected ID 'cmpl-test-123', got %q", resp.ID)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Text != " world!" {
		t.Errorf("expected ' world!', got %q", resp.Choices[0].Text)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected 'stop', got %q", resp.Choices[0].FinishReason)
	}
	if resp.Usage.TotalTokens != 5 {
		t.Errorf("expected 5 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestFIMEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := FIMResponse{
			ID:      "cmpl-empty",
			Choices: []FIMChoice{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newFIMTestClient("test-key", server.URL)

	_, err := client.FIM(context.Background(), FIMRequest{
		Model:  "deepseek-v4-pro",
		Prompt: "test",
	})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
	if !strings.Contains(err.Error(), "no choices") {
		t.Errorf("expected 'no choices' error, got: %v", err)
	}
}

func TestFIMStreaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send SSE chunks
		chunks := []string{
			`data: {"id":"cmpl-1","object":"text_completion","created":123,"model":"deepseek-v4-pro","choices":[{"text":"Hello","index":0,"finish_reason":null}]}`,
			`data: {"id":"cmpl-1","object":"text_completion","created":123,"model":"deepseek-v4-pro","choices":[{"text":" World","index":0,"finish_reason":null}]}`,
			`data: {"id":"cmpl-1","object":"text_completion","created":123,"model":"deepseek-v4-pro","choices":[{"text":"!","index":0,"finish_reason":"stop"}],"usage":{"completion_tokens":3,"prompt_tokens":1,"total_tokens":4}}`,
			`data: [DONE]`,
		}
		for _, chunk := range chunks {
			w.Write([]byte(chunk + "\n\n"))
		}
	}))
	defer server.Close()

	client := newFIMTestClient("test-key", server.URL)

	resp, err := client.FIM(context.Background(), FIMRequest{
		Model:  "deepseek-v4-pro",
		Prompt: "Say",
		Stream: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(resp.Choices[0].Text, "Hello") {
		t.Errorf("expected 'Hello' in response, got %q", resp.Choices[0].Text)
	}
	if !strings.Contains(resp.Choices[0].Text, "World") {
		t.Errorf("expected 'World' in response, got %q", resp.Choices[0].Text)
	}
	if resp.Usage.TotalTokens != 4 {
		t.Errorf("expected 4 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestFIMDefaultModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request to verify model
		var req FIMRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "deepseek-v4-pro" {
			t.Errorf("expected default model 'deepseek-v4-pro', got %q", req.Model)
		}

		resp := FIMResponse{
			ID: "cmpl-default",
			Choices: []FIMChoice{
				{Text: "ok", Index: 0},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newFIMTestClient("test-key", server.URL)

	// No model specified — should default to deepseek-v4-pro
	_, err := client.FIM(context.Background(), FIMRequest{
		Prompt: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFIMDefaultMaxTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req FIMRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.MaxTokens <= 0 {
			t.Errorf("expected non-zero max_tokens, got %d", req.MaxTokens)
		}

		resp := FIMResponse{
			ID: "cmpl-default",
			Choices: []FIMChoice{
				{Text: "ok", Index: 0},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newFIMTestClient("test-key", server.URL)

	// MaxTokens=0 should be replaced with DefaultMaxTokens
	_, err := client.FIM(context.Background(), FIMRequest{
		Model:  "deepseek-v4-pro",
		Prompt: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFIMAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid model"}`))
	}))
	defer server.Close()

	client := newFIMTestClient("test-key", server.URL)

	_, err := client.FIM(context.Background(), FIMRequest{
		Model:  "invalid-model",
		Prompt: "test",
	})
	if err == nil {
		t.Fatal("expected error for bad request")
	}
}
