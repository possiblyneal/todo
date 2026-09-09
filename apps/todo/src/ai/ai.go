// Package ai talks to the broker: an outbound OpenAI-compatible HTTP call to
// inference-runtime-broker, which already runs on the LAN. The broker is what
// infers; nothing in this repository does, and the word is deliberately not
// Agent, which CONTEXT.md gives to an Actor that writes.
//
// Nothing is inferred in this process and nothing durable is left here. A
// breakdown is a conversation that produces proposals; a proposal is a value
// that dies with the interaction, and only an approval turns one into a Task
// through the same store calls every other surface uses. There is no
// Assistance aggregate, no table, and no artifact of its own, which is what
// issue #4 settled and what keeps this an HTTP client and nothing more.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Broker is the one on the LAN. TODO_AI_URL points somewhere else, which is
// what a test and a second machine use.
const Broker = "http://10.10.10.13:4010/v1"

// Client talks to the broker. The zero Model means "whichever one the broker
// says it can chat with", asked once and kept for the run.
type Client struct {
	BaseURL string

	// Model is the one to talk to, when the caller has picked one. Empty
	// asks the broker what it is serving and settles on the first that can
	// chat.
	Model string
	HTTP  *http.Client

	// settled is that answer, kept so the question is asked once. It is
	// separate from Model, and behind a mutex, because a breakdown and an
	// `/ask` both run off the event loop and can be in flight together:
	// writing the caller's own field from two goroutines is a data race.
	mu      sync.Mutex
	settled string
}

// New reads the environment. Nothing here is configured in a file: the
// broker is a LAN address and the model is whatever it is serving today.
func New() *Client {
	c := &Client{BaseURL: Broker, Model: os.Getenv("TODO_AI_MODEL")}
	if url := os.Getenv("TODO_AI_URL"); url != "" {
		c.BaseURL = strings.TrimRight(url, "/")
	}
	return c
}

// Brief is a Task as the broker is shown it: what it says, and nothing about
// how this program stores it. No id crosses the wire, because an id means
// nothing to the broker and a proposal is matched to its parent on this side.
type Brief struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Why         string `json:"why,omitempty"`
	Deadline    string `json:"deadline,omitempty"`
	Estimate    string `json:"estimate,omitempty"`
	Priority    string `json:"priority,omitempty"`
	Impact      string `json:"impact,omitempty"`
}

// QA is one thing the broker asked and what the person answered.
type QA struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Proposal is one atomic Task the broker suggests. It is a suggestion and
// nothing else: no id, no row, and no life beyond the screen it is shown on.
type Proposal struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Why         string `json:"why"`
	Estimate    string `json:"estimate"`
	Priority    string `json:"priority"`
	Impact      string `json:"impact"`
}

// Step is one turn of a breakdown. Questions and Proposals are alternatives:
// the broker asks for what it still needs, or it has enough and proposes.
type Step struct {
	Questions []string   `json:"questions"`
	Proposals []Proposal `json:"proposals"`
}

const system = `You break a large task into atomic tasks for a todo tracker.

Answer with JSON and nothing else, shaped:
{"questions": ["..."], "proposals": [{"title": "...", "description": "...",
"why": "...", "estimate": "90m", "priority": "low|med|high",
"impact": "low|med|high"}]}

Ask questions only while you genuinely cannot break the task down: return
questions with no proposals, at most three, each answerable in a sentence.
Once you have enough, return proposals with no questions. Every proposal is
one sitting's work with a verb in its title. estimate is a Go duration such as
45m or 2h30m, or empty. Leave a field empty rather than inventing it.`

const asking = `You answer questions about somebody's todo list. You are given
the list as JSON. Answer in a few sentences of plain prose, naming tasks by
their titles. Do not invent tasks that are not in the list.`

// Breakdown takes one turn: the Task, everything already answered, and back
// comes either what the broker still needs to know or what it proposes.
func (c *Client) Breakdown(ctx context.Context, task Brief, answers []QA) (Step, error) {
	brief, _ := json.Marshal(task)
	turn := "The task:\n" + string(brief)
	if len(answers) > 0 {
		said, _ := json.Marshal(answers)
		turn += "\n\nAlready asked and answered:\n" + string(said)
	}

	said, err := c.complete(ctx, system, turn, true)
	if err != nil {
		return Step{}, err
	}
	var step Step
	if err := json.Unmarshal([]byte(object(said)), &step); err != nil {
		return Step{}, fmt.Errorf("the broker answered with something that is not a breakdown: %w", err)
	}
	return step, nil
}

// Ask answers a question about the list. The whole list goes with the
// question, because the broker holds nothing between calls.
func (c *Client) Ask(ctx context.Context, question string, list []Brief) (string, error) {
	tasks, _ := json.Marshal(list)
	return c.complete(ctx, asking, "The list:\n"+string(tasks)+"\n\nThe question: "+question, false)
}

// Models are the ids the broker says it can chat with. An embedding model is on
// the same broker and is not one of them.
func (c *Client) Models(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	var list struct {
		Data []struct {
			ID         string `json:"id"`
			Capability string `json:"capability"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("the broker listed its models as something else: %w", err)
	}
	ids := make([]string, 0, len(list.Data))
	for _, m := range list.Data {
		if m.Capability == "" || m.Capability == "chat" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

// model settles which one to talk to, once. Two callers arriving together ask
// the broker twice at worst and agree on the answer, which is the cheaper trade
// than holding the lock across the call.
func (c *Client) model(ctx context.Context) (string, error) {
	if c.Model != "" {
		return c.Model, nil
	}
	c.mu.Lock()
	settled := c.settled
	c.mu.Unlock()
	if settled != "" {
		return settled, nil
	}

	ids, err := c.Models(ctx)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("the broker is serving no model that can chat")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.settled == "" {
		c.settled = ids[0]
	}
	return c.settled, nil
}

// complete is the one call. json asks the broker for an object, which every
// OpenAI-compatible server understands and none of them guarantees, so the
// parse on the way back is lenient either way.
func (c *Client) complete(ctx context.Context, role, turn string, asJSON bool) (string, error) {
	model, err := c.model(ctx)
	if err != nil {
		return "", err
	}
	call := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": role},
			{"role": "user", "content": turn},
		},
	}
	if asJSON {
		call["response_format"] = map[string]string{"type": "json_object"}
	}
	body, err := json.Marshal(call)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")

	answer, err := c.do(req)
	if err != nil {
		return "", err
	}
	var reply struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(answer, &reply); err != nil || len(reply.Choices) == 0 {
		return "", fmt.Errorf("the broker answered with no completion in it")
	}
	return strings.TrimSpace(reply.Choices[0].Message.Content), nil
}

// do sends the request and reads the whole answer. A 429 is the broker saying
// its memory budget is spoken for, which is a wait rather than a fault, so it
// says how long.
func (c *Client) do(req *http.Request) ([]byte, error) {
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the broker at %s did not answer: %w", c.BaseURL, err)
	}
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading the broker's answer: %w", err)
	}
	if res.StatusCode == http.StatusTooManyRequests {
		var busy struct {
			Error string  `json:"error"`
			After float64 `json:"retry_after_seconds"`
		}
		_ = json.Unmarshal(body, &busy)
		return nil, fmt.Errorf("the broker is busy (%s): try again in %.0f seconds", busy.Error, busy.After)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the broker answered %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// object finds the JSON object in what came back. A broker that says "Sure!"
// and then fences its answer is still answering, and re-asking would cost
// another minute of somebody's GPU.
func object(said string) string {
	start := strings.Index(said, "{")
	end := strings.LastIndex(said, "}")
	if start < 0 || end < start {
		return said
	}
	return said[start : end+1]
}
