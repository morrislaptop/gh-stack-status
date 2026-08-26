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

func TestReorderArgs(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"17744", "--short"}, []string{"--short", "17744"}},
		{[]string{"--json", "17744"}, []string{"--json", "17744"}},
		{[]string{"-s"}, []string{"-s"}},
		{[]string{"--", "-weird-branch"}, []string{"-weird-branch"}},
	}
	for _, tc := range cases {
		got := reorderArgs(tc.in)
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Fatalf("%v: got %v want %v", tc.in, got, tc.want)
		}
	}
}

func TestTooManyArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"1", "2"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("got %v", err)
	}
}
