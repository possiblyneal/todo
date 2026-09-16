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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := http.Dir(dir).Open(r.URL.Path); err != nil {
			http.ServeFile(w, r, dir+"/index.html")
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
	switch {
	case errors.Is(err, store.ErrRefused), errors.Is(err, store.ErrHeld):
		status = http.StatusConflict
	case errors.Is(err, errUsage):
		status = http.StatusBadRequest
	}
	write(w, status, map[string]string{"error": err.Error()})
}

// errUsage marks an error the caller can fix by asking differently, which is
// exit status 2 at a terminal and 400 here.
var errUsage = errors.New("usage")
