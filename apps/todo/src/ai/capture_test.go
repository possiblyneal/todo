package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Everything the broker needs to read a dump goes with it, because it holds
// nothing between calls: the date a "tomorrow" is counted from, and the names
// it may file under. Without the date it invents one; without the names it
// invents a List.
func TestADumpCarriesTheDateAndTheNamesItMayChooseFrom(t *testing.T) {
	var sent string
	c := broker(t, func(w http.ResponseWriter, body map[string]any) {
		out, _ := json.Marshal(body["messages"])
		sent = string(out)
		completion(w, `{"title":"Call the dentist","deadline":"2026-09-10","lists":["Home"]}`)
	})

	read, err := c.Read(context.Background(), Dump{
		Text:  "call the dentist tomorrow before noon",
		Today: "2026-09-09, Wednesday",
		Lists: []string{"Home", "Work"},
		Tags:  []string{"urgent"},
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, want := range []string{"2026-09-09", "Home", "Work", "urgent", "call the dentist tomorrow"} {
		if !strings.Contains(sent, want) {
			t.Errorf("the turn did not carry %q: %s", want, sent)
		}
	}
	if read.Title != "Call the dentist" || read.Deadline != "2026-09-10" {
		t.Errorf("the capture came back %+v, want the title and the date it worked out", read)
	}
	if len(read.Lists) != 1 || read.Lists[0] != "Home" {
		t.Errorf("the capture filed it under %v, want the one List", read.Lists)
	}
}

// A dump that amends a Task carries the Task as it stands, and what comes back
// is the whole of it rather than a difference: nothing on this side could
// apply a difference to attributes it was not told about.
func TestADumpThatAmendsATaskCarriesTheTaskAsItStands(t *testing.T) {
	var sent string
	c := broker(t, func(w http.ResponseWriter, body map[string]any) {
		out, _ := json.Marshal(body["messages"])
		sent = string(out)
		completion(w, `{"title":"Fix the roof","why":"Water is getting in","priority":"high"}`)
	})

	read, err := c.Read(context.Background(), Dump{
		Text:  "make it high priority",
		Today: "2026-09-09, Wednesday",
		Was:   &Capture{Title: "Fix the roof", Why: "Water is getting in"},
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(sent, "Fix the roof") || !strings.Contains(sent, "Water is getting in") {
		t.Errorf("the turn did not carry the Task as it stands: %s", sent)
	}
	if read.Title != "Fix the roof" || read.Why != "Water is getting in" || read.Priority != "high" {
		t.Errorf("the capture came back %+v, want the whole Task with the one change", read)
	}
}

// The lenient parse is the same one every other answer gets: a broker that
// says "Sure!" and fences its JSON is still answering, and re-asking costs
// another minute of somebody's GPU.
func TestACaptureInACodeFenceIsStillACapture(t *testing.T) {
	c := broker(t, func(w http.ResponseWriter, body map[string]any) {
		completion(w, "Sure!\n```json\n{\"title\":\"Rake the leaves\"}\n```\n")
	})

	read, err := c.Read(context.Background(), Dump{Text: "rake the leaves", Today: "2026-09-09, Wednesday"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if read.Title != "Rake the leaves" {
		t.Errorf("the capture came back %+v, want the title inside the fence", read)
	}
}

func TestSomethingThatIsNotATaskIsAnError(t *testing.T) {
	c := broker(t, func(w http.ResponseWriter, body map[string]any) {
		completion(w, "I could not read that.")
	})

	if _, err := c.Read(context.Background(), Dump{Text: "…", Today: "2026-09-09, Wednesday"}); err == nil {
		t.Error("prose was accepted as a task")
	}
}
