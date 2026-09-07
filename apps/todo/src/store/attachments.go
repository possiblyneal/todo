package store

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// An Attachment is a pointer a Task holds to something living outside the
// tracker: a file path or a web address. The three kinds docs/features.md
// groups -- attachments, websites, file locations -- are one thing here,
// because to the store they are all a piece of text naming somewhere else.
//
// Nothing is copied in, nothing is stored but the pointer, and nothing is
// collected when a Task is deleted. A target that moves or goes leaves a dead
// link and the tracker cannot tell: it never looks, and there is deliberately
// no checker. That was decided at Aggregates, issue #3, and the whole cost of
// deciding it is that a pointer is cheap.

// Attach points a Task at something outside the tracker. It is a write to the
// Task, so it needs the Lease covering the Task's tree, and it does not check
// that the target exists.
func (s *Store) Attach(actor, taskID, target string) error {
	target, err := pointer(target)
	if err != nil {
		return err
	}
	return s.guarded(actor, taskID, KindAttachmentAdded, map[string]any{"target": target})
}

// Detach takes a pointer off a Task. What it pointed at is not the tracker's
// to touch, so nothing else happens.
func (s *Store) Detach(actor, taskID, target string) error {
	target, err := pointer(target)
	if err != nil {
		return err
	}
	return s.guarded(actor, taskID, KindAttachmentRemoved, map[string]any{"target": target})
}

// pointer is how a target is written down. A web address is kept as typed; a
// path is resolved against the directory it was typed in, so the same pointer
// read from anywhere else still names the same file.
func pointer(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("an Attachment needs somewhere to point")
	}
	if u, err := url.Parse(target); err == nil && u.Scheme != "" && u.Host != "" {
		return target, nil
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("%q is not somewhere: %w", target, err)
	}
	return abs, nil
}
