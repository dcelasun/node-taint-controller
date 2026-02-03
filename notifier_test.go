package main

import (
	"os"
	"testing"
)

func TestNewNotifierFromEnv_Empty(t *testing.T) {
	os.Unsetenv("SHOUTRRR_URLS")
	n := NewNotifierFromEnv()
	if n != nil {
		t.Error("expected nil notifier when SHOUTRRR_URLS is empty")
	}
}

func TestNewNotifierFromEnv_Whitespace(t *testing.T) {
	os.Setenv("SHOUTRRR_URLS", "  ,  ,  ")
	defer os.Unsetenv("SHOUTRRR_URLS")

	n := NewNotifierFromEnv()
	if n != nil {
		t.Error("expected nil notifier when SHOUTRRR_URLS contains only whitespace")
	}
}

func TestNotifier_NilSafe(t *testing.T) {
	var n *Notifier

	// These should not panic
	n.Send("test")
	n.Sendf("test %s", "foo")
	n.TaintAdded("node1")
	n.TaintRemoved("node1")
	n.Error("context", nil)
}
