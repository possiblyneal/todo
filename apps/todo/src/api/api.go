// Package api is the JSON surface the browser client reads the tracker
// through. It holds routing, encoding and the status code an error takes, and
// no rules: every handler is another caller of the same store the verbs call,
// so a person on a phone and an Agent at a terminal get the same contract.
//
// See docs/adrs/0003-replace-the-tui-with-a-browser-client.md and
// docs/plans/browser-client.md.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// Options is how the listener is configured. Web is the directory of compiled
// client files served beside the JSON; empty serves the JSON alone, which is
// what a client running its own dev server wants.
type Options struct {
	Addr string
	Web  string
}

// Handler is every route, and it is what a test exercises without a listener.
func Handler(s *store.Store, web string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		state(s, w, r)
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
	if web != "" {
		mux.Handle("/", client(web))
	}
	return mux
}

// ListenAndServe opens the listener. The address is the caller's, and ADR
// 0003 binds it to the LAN: there is no authentication here on purpose, so an
// address reachable from outside the LAN is a decision that record's first
// re-check trigger covers.
func ListenAndServe(s *store.Store, o Options, stderr io.Writer) error {
	fmt.Fprintf(stderr, "todo api: listening on %s\n", o.Addr)
	srv := &http.Server{Addr: o.Addr, Handler: Handler(s, o.Web)}
	return srv.ListenAndServe()
}

// client serves the compiled client, falling back to index.html for a path
// that names no file. The client routes in the browser, so a reload on a path
// only it knows about has to reach it rather than 404.
func client(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Stat rather than open: the answer wanted is whether the file is
		// there, and an opened one would have to be closed on a path that
		// hands the serving to somebody who opens it again anyway.
		named := filepath.Join(dir, filepath.FromSlash(path.Clean("/"+r.URL.Path)))
		if info, err := os.Stat(named); err != nil || info.IsDir() {
			http.ServeFile(w, r, index)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// write sends one JSON body. An encoding failure after the status is written
// cannot be reported to the client, so it is logged nowhere and dropped: the
// response is already short by then and the client's next poll replaces it.
func write(w http.ResponseWriter, status int, body any) {
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
	case errors.Is(err, store.ErrRefused), errors.Is(err, store.ErrHeld):
		status = http.StatusConflict
	case errors.As(err, &asked):
		status = http.StatusBadRequest
	}
	write(w, status, map[string]string{"error": err.Error()})
}

// usage marks an error the caller can fix by asking differently, which is exit
// status 2 at a terminal and 400 here. It adds no words of its own: the body
// is the sentence the CLI would have printed and nothing more, so a person
// reading the browser and a person reading the terminal are told the same
// thing about the same mistake.
type usage struct{ err error }

func (u usage) Error() string { return u.err.Error() }
func (u usage) Unwrap() error { return u.err }
