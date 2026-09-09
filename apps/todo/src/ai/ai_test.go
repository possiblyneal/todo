package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// broker stands in for inference-runtime-broker. Every test here is about what
// this program sends and what it makes of what comes back; the broker itself is
// somebody else's deployable and is not started by a test.
func broker(t *testing.T, reply func(w http.ResponseWriter, body map[string]any)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[
				{"id":"qwen3-embedding","capability":"embedding"},
				{"id":"qwen3.8-flash-next","capability":"chat"}]}`))
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("the client asked for %s, want an OpenAI-compatible path", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("the request body was not JSON: %v", err)
		}
		w.Header().Set("content-type", "application/json")
		reply(w, body)
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL + "/v1", HTTP: srv.Client()}
}

// completion is what an OpenAI-compatible broker answers with.
func completion(w http.ResponseWriter, content string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{"content": content}}},
	})
}

func TestTheModelIsTheOneTheBoxSaysItCanChatWith(t *testing.T) {
	var asked string
	c := broker(t, func(w http.ResponseWriter, body map[string]any) {
		asked, _ = body["model"].(string)
		completion(w, `{"questions":[],"proposals":[]}`)
	})

	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 1 || models[0] != "qwen3.8-flash-next" {
		t.Errorf("the broker offered %v, want only the chat model", models)
	}
	if _, err := c.Breakdown(context.Background(), Brief{Title: "Paint the shed"}, nil); err != nil {
		t.Fatalf("Breakdown: %v", err)
	}
	if asked != "qwen3.8-flash-next" {
		t.Errorf("the call named model %q, want the one the broker said it could chat with", asked)
	}
}

func TestAskingForMoreComesBackAsQuestions(t *testing.T) {
	c := broker(t, func(w http.ResponseWriter, body map[string]any) {
		completion(w, `{"questions":["What colour?","Which shed?"],"proposals":[]}`)
	})

	step, err := c.Breakdown(context.Background(), Brief{Title: "Paint the shed"}, nil)
	if err != nil {
		t.Fatalf("Breakdown: %v", err)
	}
	if len(step.Questions) != 2 || len(step.Proposals) != 0 {
		t.Fatalf("the step came back %+v, want the two questions and nothing else", step)
	}
}

// What was answered goes back with the next turn. Without it the broker asks
// the same question forever, because nothing here keeps a conversation for
// it.
func TestTheAnswersGoBackWithTheNextTurn(t *testing.T) {
	var sent string
	c := broker(t, func(w http.ResponseWriter, body map[string]any) {
		out, _ := json.Marshal(body["messages"])
		sent = string(out)
		completion(w, `{"proposals":[{"title":"Sand it","estimate":"90m","priority":"med"}]}`)
	})

	step, err := c.Breakdown(context.Background(), Brief{Title: "Paint the shed"},
		[]QA{{Question: "What colour?", Answer: "Green"}})
	if err != nil {
		t.Fatalf("Breakdown: %v", err)
	}
	if !strings.Contains(sent, "What colour?") || !strings.Contains(sent, "Green") {
		t.Errorf("the turn carried %s, want the question and its answer", sent)
	}
	if len(step.Proposals) != 1 || step.Proposals[0].Title != "Sand it" {
		t.Fatalf("the step came back %+v, want the one proposal", step)
	}
	if step.Proposals[0].Estimate != "90m" || step.Proposals[0].Priority != "med" {
		t.Errorf("the proposal came back %+v, want its attributes", step.Proposals[0])
	}
}

// A broker that wraps its JSON in a code fence is still answering. Nothing here
// runs a grammar on the far end, so the parse is the lenient one.
func TestJSONInACodeFenceIsStillJSON(t *testing.T) {
	c := broker(t, func(w http.ResponseWriter, body map[string]any) {
		completion(w, "Sure!\n```json\n{\"proposals\":[{\"title\":\"Sand it\"}]}\n```\n")
	})

	step, err := c.Breakdown(context.Background(), Brief{Title: "Paint the shed"}, nil)
	if err != nil {
		t.Fatalf("Breakdown: %v", err)
	}
	if len(step.Proposals) != 1 {
		t.Fatalf("the fenced answer parsed to %+v, want the one proposal", step)
	}
}

// The broker is shared and its memory budget is finite: it answers 429 with how
// long to wait. That is a sentence to read, not a stack trace.
func TestABusyBoxSaysWhenToComeBack(t *testing.T) {
	c := broker(t, func(w http.ResponseWriter, body map[string]any) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"scheduler_busy","retry_after_seconds":30}`))
	})

	_, err := c.Breakdown(context.Background(), Brief{Title: "Paint the shed"}, nil)
	if err == nil {
		t.Fatal("a busy broker came back as success")
	}
	if !strings.Contains(err.Error(), "busy") || !strings.Contains(err.Error(), "30") {
		t.Errorf("a busy broker read %q, want it to say it is busy and for how long", err)
	}
}

func TestAQuestionAboutTheListIsAnsweredInProse(t *testing.T) {
	var sent string
	c := broker(t, func(w http.ResponseWriter, body map[string]any) {
		out, _ := json.Marshal(body["messages"])
		sent = string(out)
		completion(w, "The roof, then the shed.")
	})

	answer, err := c.Ask(context.Background(), "What should I do first?", []Brief{
		{Title: "Fix the roof", Priority: "high"},
		{Title: "Paint the shed"},
	})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if answer != "The roof, then the shed." {
		t.Errorf("the answer came back %q", answer)
	}
	if !strings.Contains(sent, "Fix the roof") || !strings.Contains(sent, "Paint the shed") {
		t.Errorf("the question carried %s, want the list it is about", sent)
	}
}

// TestTwoCallsAtOnceSettleOnOneModel is the client as the screen holds it:
// one, shared, with a breakdown and an `/ask` able to be in flight together.
// Run under -race.
func TestTwoCallsAtOnceSettleOnOneModel(t *testing.T) {
	var mu sync.Mutex
	asked := map[string]int{}
	c := broker(t, func(w http.ResponseWriter, body map[string]any) {
		mu.Lock()
		asked[body["model"].(string)]++
		mu.Unlock()
		completion(w, `{"questions":[],"proposals":[]}`)
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Breakdown(context.Background(), Brief{Title: "Paint the shed"}, nil); err != nil {
				t.Errorf("Breakdown: %v", err)
			}
		}()
	}
	wg.Wait()

	if len(asked) != 1 {
		t.Errorf("the calls named %v, want every one of them on the same model", asked)
	}
}
