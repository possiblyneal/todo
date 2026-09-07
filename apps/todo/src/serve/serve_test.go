package serve

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/wish/v2/testsession"
	gossh "golang.org/x/crypto/ssh"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// keypair makes one client key. Two of them is how a test tells an allowed key
// from a refused one without any of this depending on the machine it runs on.
func keypair(t *testing.T) (gossh.Signer, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}
	line, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	return signer, string(gossh.MarshalAuthorizedKey(line))
}

// fixture is a store with one Task in it, which is enough to see something
// rendered and to have something to write to.
func fixture(t *testing.T) (*store.Store, string) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "todo.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	id, err := s.AddTask("alice", store.Attributes{Title: store.Set("Fix the roof")})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	return s, id
}

// options points the server at an allowlist holding exactly the given keys.
func options(t *testing.T, keys ...string) Options {
	t.Helper()
	dir := t.TempDir()
	allowed := filepath.Join(dir, "authorized_keys")
	if err := writeFile(allowed, strings.Join(keys, "")); err != nil {
		t.Fatalf("writing authorized_keys: %v", err)
	}
	return Options{
		Addr:           "127.0.0.1:0",
		HostKey:        filepath.Join(dir, "ssh_host_ed25519"),
		AuthorizedKeys: allowed,
	}
}

func client(user string, auth ...gossh.AuthMethod) *gossh.ClientConfig {
	return &gossh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
}

// TestAPublicKeyIsTheOnlyWayIn is the constraint the whole seam rests on. The
// key on the allowlist gets a session; another key does not, and neither does
// a password, because no password handler is set at all.
func TestAPublicKeyIsTheOnlyWayIn(t *testing.T) {
	s, _ := fixture(t)
	mine, line := keypair(t)
	theirs, _ := keypair(t)

	srv, err := Server(s, options(t, line))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	addr := testsession.Listen(t, srv)

	if _, err := testsession.NewClientSession(t, addr, client("alice", gossh.PublicKeys(mine))); err != nil {
		t.Fatalf("the allowed key was refused: %v", err)
	}
	if _, err := testsession.NewClientSession(t, addr, client("mallory", gossh.PublicKeys(theirs))); err == nil {
		t.Error("a key that is not on the allowlist got in")
	}
	if _, err := testsession.NewClientSession(t, addr, client("mallory", gossh.Password("hunter2"))); err == nil {
		t.Error("a password got in")
	}
}

// TestNoAllowlistMeansNoServer: starting with nothing to check keys against
// would be a server anyone can reach, so it is a refusal to listen.
func TestNoAllowlistMeansNoServer(t *testing.T) {
	s, _ := fixture(t)
	o := options(t)
	o.AuthorizedKeys = filepath.Join(t.TempDir(), "nothing-here")

	if _, err := Server(s, o); err == nil {
		t.Fatal("a server started with no authorized_keys")
	} else if !strings.Contains(err.Error(), "public key") {
		t.Errorf("the refusal read %q, want it to say a public key is the only way in", err)
	}
}

// TestASessionWithNoTerminalIsTurnedAway. `ssh host todo` with no pty has
// nowhere to draw, so activeterm says so and the session ends rather than a
// renderer writing escape codes into a pipe.
func TestASessionWithNoTerminalIsTurnedAway(t *testing.T) {
	s, _ := fixture(t)
	mine, line := keypair(t)

	srv, err := Server(s, options(t, line))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	sess := testsession.New(t, srv, client("alice", gossh.PublicKeys(mine)))

	out, err := sess.Output("")
	if err == nil {
		t.Error("a session with no pty was served a TUI")
	}
	if !strings.Contains(string(out), "PTY") {
		t.Errorf("the session was told %q, want it to name the missing terminal", out)
	}
}

// TestThePhoneReadsAndWrites is the ticket in one test: a session over the
// wire sees the Tasks that are there, and a command run in it lands in the
// same SQLite file this process is holding open.
func TestThePhoneReadsAndWrites(t *testing.T) {
	s, id := fixture(t)
	mine, line := keypair(t)

	srv, err := Server(s, options(t, line))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	sess := testsession.New(t, srv, client("alice", gossh.PublicKeys(mine)))

	stdin, err := sess.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	var out screen
	sess.Stdout = &out
	if err := sess.RequestPty("xterm-256color", 40, 120, nil); err != nil {
		t.Fatalf("RequestPty: %v", err)
	}
	if err := sess.Shell(); err != nil {
		t.Fatalf("Shell: %v", err)
	}

	waitFor(t, func() bool { return strings.Contains(out.String(), "Fix the roof") },
		"the Task never reached the far end of the wire")

	// The palette, the command, and enter: the same keystrokes a person makes,
	// arriving as bytes over SSH rather than from a local terminal.
	if _, err := io.WriteString(stdin, "/complete\r"); err != nil {
		t.Fatalf("writing keystrokes: %v", err)
	}

	waitFor(t, func() bool {
		tasks, err := s.Tasks(store.Query{IncludeCompleted: true})
		if err != nil {
			t.Fatalf("Tasks: %v", err)
		}
		for _, got := range tasks {
			if got.ID == id && !got.CompletedAt.IsZero() {
				return true
			}
		}
		return false
	}, "a command run over SSH never reached the store")
}

// TestRotatingThePhoneRedrawsToFit. A window-change arrives on its own channel
// mid-session, and the middleware forwards it as a resize; without that the
// program would draw at whatever size the pty was requested with forever.
func TestRotatingThePhoneRedrawsToFit(t *testing.T) {
	s, _ := fixture(t)
	mine, line := keypair(t)

	srv, err := Server(s, options(t, line))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	sess := testsession.New(t, srv, client("alice", gossh.PublicKeys(mine)))

	var out screen
	sess.Stdout = &out
	if err := sess.RequestPty("xterm-256color", 40, 120, nil); err != nil {
		t.Fatalf("RequestPty: %v", err)
	}
	if err := sess.Shell(); err != nil {
		t.Fatalf("Shell: %v", err)
	}
	waitFor(t, func() bool { return widest(out.String()) > 40 },
		"nothing was drawn at the size the pty was asked for")

	out.reset()
	if err := sess.WindowChange(20, 40); err != nil {
		t.Fatalf("WindowChange: %v", err)
	}
	waitFor(t, func() bool { drawn := out.String(); return drawn != "" && widest(drawn) <= 40 },
		"the session kept drawing at the old size after a resize")
}
