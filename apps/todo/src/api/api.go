// Package api is the JSON surface the browser client reads the tracker
// through. It holds routing, encoding and the status code an error takes, and
// no rules: every handler is another caller of the same store the verbs call,
// so a person on a phone and an Agent at a terminal get the same contract.
// GET /api/files is the one exception and says so where it is registered: it
// reads a directory rather than the store, so the root it will not look above
// is a rule with nowhere else to live.
//
// See docs/adrs/0003-replace-the-tui-with-a-browser-client.md and
// docs/plans/browser-client.md.
package api

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/possiblyneal/todo/apps/todo/src/ai"
	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// Options is how the listener is configured. Web is the directory of compiled
// client files served beside the JSON; empty serves the JSON alone, which is
// what a client running its own dev server wants.
//
// Actor is who every write through this listener is attributed to. There is no
// authentication here and so nobody to name per request: `todo api` resolves
// its Actor once, the same way a verb resolves one, and the whole listener
// writes as that. A browser on the LAN is the person who started it.
// Browse is the directory the file picker lists from and will not look above.
// Empty is the home directory of the user running `todo api`, which is the
// machine a pointer's relative path resolves against.
type Options struct {
	Addr   string
	Web    string
	Actor  string
	Browse string
}

// Handler is every route, and it is what a test exercises without a listener.
func Handler(s *store.Store, o Options) http.Handler {
	// One client for the process, so the question of which model the broker is
	// serving is asked once rather than once a request.
	broker := ai.New()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		state(s, w, r)
	})
	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		addTask(s, o.Actor, w, r)
	})
	mux.HandleFunc("PATCH /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		editTask(s, o.Actor, w, r)
	})
	mux.HandleFunc("POST /api/tasks/{id}/subtasks", func(w http.ResponseWriter, r *http.Request) {
		addSubtask(s, o.Actor, w, r)
	})
	// The pointers a Task holds. Both carry the target in the body, so the two
	// are one path and differ by method the way the collections' do.
	mux.HandleFunc("POST /api/tasks/{id}/attachments", func(w http.ResponseWriter, r *http.Request) {
		attach(s, o.Actor, w, r)
	})
	mux.HandleFunc("DELETE /api/tasks/{id}/attachments", func(w http.ResponseWriter, r *http.Request) {
		detach(s, o.Actor, w, r)
	})
	// The four lifecycle verbs, by name in the path. A literal segment beats a
	// wildcard one in this mux, so the route above is what serves `subtasks`
	// and this one never sees it.
	mux.HandleFunc("POST /api/tasks/{id}/{verb}", func(w http.ResponseWriter, r *http.Request) {
		lifecycleTask(s, o.Actor, w, r)
	})
	// Scheduling: the rule as one value, and the three marks against one date.
	// The rule's three methods sit under the same path because a Series is one
	// thing a Task either has or does not, and the marks are a segment deeper
	// because they are about a date rather than about the rule.
	mux.HandleFunc("GET /api/tasks/{id}/series", func(w http.ResponseWriter, r *http.Request) {
		series(s, w, r)
	})
	mux.HandleFunc("PUT /api/tasks/{id}/series", func(w http.ResponseWriter, r *http.Request) {
		repeatSeries(s, o.Actor, w, r)
	})
	mux.HandleFunc("DELETE /api/tasks/{id}/series", func(w http.ResponseWriter, r *http.Request) {
		unrepeatSeries(s, o.Actor, w, r)
	})
	mux.HandleFunc("POST /api/tasks/{id}/series/{mark}", func(w http.ResponseWriter, r *http.Request) {
		markOccurrence(s, o.Actor, w, r)
	})
	// The fourth thing done to a date, and the one that carries a Task rather
	// than only the date: the literal segment takes precedence over {mark}.
	mux.HandleFunc("POST /api/tasks/{id}/series/edit", func(w http.ResponseWriter, r *http.Request) {
		detachEdited(s, o.Actor, w, r)
	})
	mux.HandleFunc("GET /api/tasks/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		taskHistory(s, w, r)
	})
	mux.HandleFunc("GET /api/history", func(w http.ResponseWriter, r *http.Request) {
		history(s, w, r)
	})
	// A List and a Tag are the same three writes against different aggregates,
	// so the routes are registered from one table and differ only in the pair
	// of store calls they are handed.
	for path, of := range map[string]func(*store.Store) kind{"lists": lists, "tags": tags} {
		k := of(s)
		mux.HandleFunc("POST /api/"+path, func(w http.ResponseWriter, r *http.Request) {
			addCollection(k, o.Actor, w, r)
		})
		mux.HandleFunc("PATCH /api/"+path+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			describeCollection(k, o.Actor, w, r)
		})
		mux.HandleFunc("DELETE /api/"+path+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			dropCollection(k, o.Actor, w, r)
		})
	}
	// The one route reading outside the store. It lists and never opens, and
	// what it lists is the machine a pointer resolves against rather than the
	// phone the pointer is being typed into.
	root := rooted(cmp.Or(o.Browse, home()))
	mux.HandleFunc("GET /api/files", func(w http.ResponseWriter, r *http.Request) {
		browse(root, w, r)
	})
	mux.HandleFunc("POST /api/capture", func(w http.ResponseWriter, r *http.Request) {
		capture(s, broker, w, r)
	})
	mux.HandleFunc("POST /api/ask", func(w http.ResponseWriter, r *http.Request) {
		ask(s, broker, w, r)
	})
	mux.HandleFunc("POST /api/breakdown", func(w http.ResponseWriter, r *http.Request) {
		breakdown(s, broker, w, r)
	})
	// A route under /api/ that this package does not serve is a usage error in
	// the same envelope every other one arrives in. It is registered whether or
	// not the client is served here: with the files under it the fallback below
	// would otherwise hand back index.html at 200 for the client to fail to
	// parse as JSON, and without them a bad route would answer this in one
	// posture and Go's own plain-text 404 in the other.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, usage{fmt.Errorf("%s %s is not a route", r.Method, r.URL.Path)})
	})
	if o.Web != "" {
		mux.Handle("/", client(o.Web))
	}
	return mux
}

// ListenAndServe opens the listener. The address is the caller's, and ADR
// 0003 binds it to the LAN: there is no authentication here on purpose, so an
// address reachable from outside the LAN is a decision that record's first
// re-check trigger covers.
func ListenAndServe(s *store.Store, o Options, stderr io.Writer) error {
	fmt.Fprintf(stderr, "todo api: listening on %s\n", o.Addr)
	srv := &http.Server{Addr: o.Addr, Handler: Handler(s, o)}
	return srv.ListenAndServe()
}

// client serves the compiled client, falling back to index.html for a path
// that names no file. The client routes in the browser, so a reload on a path
// only it knows about has to reach it rather than 404.
func client(dir string) http.Handler {
	root := http.Dir(dir)
	files := http.FileServer(root)
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The same http.Dir decides whether the file is there and then serves
		// it, so the two cannot disagree about which file a request names, and
		// the one check that keeps a request inside the directory is the
		// standard library's rather than a second one written here to match.
		file, err := root.Open(r.URL.Path)
		if err != nil {
			http.ServeFile(w, r, index)
			return
		}
		info, err := file.Stat()
		_ = file.Close()
		if err != nil || info.IsDir() {
			http.ServeFile(w, r, index)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// send writes one JSON body. An encoding failure after the status is written
// cannot be reported to the client, so it is logged nowhere and dropped: the
// response is already short by then and the client's next poll replaces it.
func send(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// fail maps an error onto the status carrying the meaning the CLI's exit
// status carries: a refusal is not a failure, and neither is a bad request.
// The body is the sentence the CLI would have printed.
func fail(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var asked usage
	switch {
	case store.Refused(err):
		status = http.StatusConflict
	case errors.As(err, &asked):
		status = http.StatusBadRequest
	}
	send(w, status, map[string]string{"error": err.Error()})
}

// page reads the `limit` query parameter every route that answers a page of
// something reads: a whole number of at least one, capped at what that route
// will answer, and the route's own default where the caller says nothing. The
// word is the same on every route because it is the same question, and the
// sentence names what is being counted so the answer is about dates or entries
// rather than about a number.
func page(r *http.Request, of string, fallback, limit int) (int, error) {
	asked := r.URL.Query().Get("limit")
	if asked == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(asked)
	if err != nil || n < 1 {
		return 0, usage{fmt.Errorf("cannot read %q as how many %s to read: want a whole number of at least 1", asked, of)}
	}
	return min(n, limit), nil
}

// usage marks an error the caller can fix by asking differently, which is exit
// status 2 at a terminal and 400 here. It adds no words of its own: the body
// is the sentence the CLI would have printed and nothing more, so a person
// reading the browser and a person reading the terminal are told the same
// thing about the same mistake.
type usage struct{ err error }

func (u usage) Error() string { return u.err.Error() }
func (u usage) Unwrap() error { return u.err }
