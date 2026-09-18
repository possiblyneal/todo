package api

import (
	"net/http"
	"slices"
	"testing"
)

func TestAttachPointsTheTaskAtSomewhereElse(t *testing.T) {
	s := openTemp(t)
	id := add(t, s, "Read the paper")

	w := do(t, s, "POST", "/api/tasks/"+id+"/attachments", `{"target":"https://example.com/x"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (%s)", w.Code, w.Body.String())
	}
	if got := only(t, s).Attachments; !slices.Equal(got, []string{"https://example.com/x"}) {
		t.Fatalf("attachments %v, want the one pointed at", got)
	}
}

func TestDetachTakesThePointerOffAgain(t *testing.T) {
	s := openTemp(t)
	id := add(t, s, "Read the paper")
	do(t, s, "POST", "/api/tasks/"+id+"/attachments", `{"target":"https://example.com/x"}`)

	w := do(t, s, "DELETE", "/api/tasks/"+id+"/attachments", `{"target":"https://example.com/x"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (%s)", w.Code, w.Body.String())
	}
	if got := only(t, s).Attachments; len(got) != 0 {
		t.Fatalf("attachments %v, want none", got)
	}
}

// `attachments` is a literal segment and the lifecycle route is a wildcard one,
// so the two share a shape and the mux is what keeps them apart. A body with
// no target is refused as usage rather than reaching the store.
func TestAttachRefusesAPointerToNowhere(t *testing.T) {
	s := openTemp(t)
	id := add(t, s, "Read the paper")

	w := do(t, s, "POST", "/api/tasks/"+id+"/attachments", `{"target":""}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400 (%s)", w.Code, w.Body.String())
	}
}

func TestAttachingIsNotMistakenForALifecycleVerb(t *testing.T) {
	s := openTemp(t)
	id := add(t, s, "Read the paper")

	do(t, s, "POST", "/api/tasks/"+id+"/attachments", `{"target":"https://example.com/x"}`)
	if got := only(t, s).CompletedAt; !got.IsZero() {
		t.Fatalf("the task ended at %v, want the attachment route to have served it", got)
	}
}
