package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
	"github.com/possiblyneal/todo/apps/todo/src/write"
)

// capture is POST /api/capture: somebody's words read into a Task, and nothing
// written. What comes back is what the Broker said, in the shape POST
// /api/tasks takes, so the client fills the add sheet with it and submitting
// is the only thing that writes. A dump read and then abandoned leaves nothing
// behind, which is what keeps the form the gate on a surface a person is at.
//
// The attributes come back as the Broker said them rather than parsed. A
// duration it wrote in a way this program cannot read belongs in the field for
// somebody to correct, not dropped on the way to a screen they are looking at:
// dropping it is `todo capture`'s rule, and that verb has nobody left to ask.
func capture(s *store.Store, c *ai.Client, w http.ResponseWriter, r *http.Request) {
	in, err := decode[captureBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		fail(w, usage{errors.New("say what the task is")})
		return
	}

	dump, err := write.Gather(s, text)
	if err != nil {
		fail(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), ai.Patience)
	defer cancel()
	read, err := c.Read(ctx, dump.Shown)
	if err != nil {
		fail(w, err)
		return
	}

	// The Lists and Tags it chose are answered by id rather than by the names
	// it chose them under, because an id is what the write takes and matching
	// a name back to one is this side's job either way. Which of its answers
	// is which attribute is write.AsSaid's, the same as it is FromCapture's.
	send(w, http.StatusOK, saying(write.AsSaid(read), dump.Filed(read)))
}

// ask is POST /api/ask: a question about the Tasks in view, answered as prose.
// It is a read like the list it is about: no Lease, nothing appended, and
// nothing kept between calls, which is why the whole list goes with every
// question.
//
// Which Tasks are in view is the query string, read the way GET /api/state
// reads it, so a question asked under a filter is asked about what the filter
// left on the screen.
func ask(s *store.Store, c *ai.Client, w http.ResponseWriter, r *http.Request) {
	in, err := decode[askBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	question := strings.TrimSpace(in.Question)
	if question == "" {
		fail(w, usage{errors.New("say what the question is")})
		return
	}

	tasks, err := s.Tasks(query(r))
	if err != nil {
		fail(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), ai.Patience)
	defer cancel()
	answer, err := c.Ask(ctx, question, write.Briefs(tasks))
	if err != nil {
		fail(w, err)
		return
	}
	send(w, http.StatusOK, map[string]string{"answer": answer})
}

// captureBody is the dump: the words somebody typed, and nothing else. What
// the Broker may file the Task under is the store's answer rather than the
// client's, so it is gathered here.
type captureBody struct {
	Text string `json:"text"`
}

// askBody is the question. The Tasks it is about are the query string's.
type askBody struct {
	Question string `json:"question"`
}

// breakdown is POST /api/breakdown: one turn of a breakdown. The Task goes to
// the Broker with everything already answered, and back comes either what it
// still needs to know or what it proposes. Like the other two Broker routes it
// writes nothing and takes no Lease.
//
// The TUI holds a Lease over the tree for the whole interaction, because there
// the interaction is one screen with a person sitting at it. Here there is no
// interaction to hold one across: each turn is a request that ends, and the
// proposals live on the client until somebody approves them. So an approved
// proposal is written by POST /api/tasks/{id}/subtasks like any other Subtask,
// under the Lease that write takes for itself, and a tree that moved while the
// proposals were being read is the same thing that can happen to an add sheet
// left open.
//
// Nothing here carries a position. Which proposals were approved is the
// client's to remember, because two proposals may come back saying the same
// thing and only the order tells them apart; what reaches this side is the
// bodies of the ones ticked, one write each.
func breakdown(s *store.Store, c *ai.Client, w http.ResponseWriter, r *http.Request) {
	in, err := decode[breakdownBody](w, r)
	if err != nil {
		fail(w, usage{err})
		return
	}
	if strings.TrimSpace(in.Task) == "" {
		fail(w, usage{errors.New("say which task is being broken down")})
		return
	}
	brief, err := write.BriefOf(s, in.Task)
	if err != nil {
		// A Task the list does not name is the caller asking about one that is
		// not there, which is the same 400 an unknown route gets.
		fail(w, usage{err})
		return
	}

	answers := make([]ai.QA, 0, len(in.Answers))
	for _, said := range in.Answers {
		answers = append(answers, ai.QA{Question: said.Question, Answer: said.Answer})
	}
	ctx, cancel := context.WithTimeout(r.Context(), ai.Patience)
	defer cancel()
	step, err := c.Breakdown(ctx, brief, answers)
	if err != nil {
		fail(w, err)
		return
	}
	// A turn with neither half is the Broker having answered nothing usable.
	// It is not this side failing and not the caller asking wrongly, which is
	// what 500 means here: the same sentence the TUI ends a breakdown on.
	if len(step.Questions) == 0 && len(step.Proposals) == 0 {
		fail(w, errors.New("the broker had nothing to ask and nothing to propose"))
		return
	}

	out := stepBody{Questions: step.Questions, Proposals: []taskBody{}}
	for _, p := range step.Proposals {
		// The same shape POST /api/capture answers in, because a proposal is
		// corrected and submitted the way a dump read into a Task is.
		out.Proposals = append(out.Proposals, saying(write.AsProposed(p), write.Membership{}))
	}
	send(w, http.StatusOK, out)
}

// breakdownBody is the Task being broken down and everything already asked and
// answered. The Broker holds nothing between calls, so every turn carries the
// whole conversation.
type breakdownBody struct {
	Task    string `json:"task"`
	Answers []struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	} `json:"answers,omitempty"`
}

// stepBody is one turn coming back. The two halves are alternatives: the
// Broker asks for what it still needs, or it has enough and proposes.
type stepBody struct {
	Questions []string   `json:"questions,omitempty"`
	Proposals []taskBody `json:"proposals"`
}
