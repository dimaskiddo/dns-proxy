package main

import "testing"

// TestRunServerBadConfigPathReturnsError pins §1.10: a bad config path must
// return an error from runServer, not exit the process via log.Fatalf.
func TestRunServerBadConfigPathReturnsError(t *testing.T) {
	err := runServer("./does-not-exist.yaml")
	if err == nil {
		t.Fatal("expected an error for a nonexistent config path, got nil")
	}
}
