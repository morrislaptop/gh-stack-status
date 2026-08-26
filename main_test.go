package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "gh stack-status") {
		t.Fatalf("help on stderr: %q", stderr.String())
	}
}

func TestTooManyArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"1", "2"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("got %v", err)
	}
}
