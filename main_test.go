package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/morrislaptop/gh-stack-status/internal/github"
	"github.com/morrislaptop/gh-stack-status/internal/render"
	"github.com/morrislaptop/gh-stack-status/internal/resolve"
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

type stubGQL struct {
	routes map[string]string
}

func (s stubGQL) Do(query string, variables map[string]interface{}, response interface{}) error {
	for marker, body := range s.routes {
		if strings.Contains(query, marker) {
			return json.Unmarshal([]byte(body), response)
		}
	}
	return fmt.Errorf("no route for query: %s", query)
}

type stubREST struct{}

func (stubREST) Request(method, path string, body io.Reader) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)),
		Header:     make(http.Header),
	}, nil
}

// GitHub offers to rebase a stack that has fallen behind its base branch while
// still reporting the merge state as CLEAN, so the bottom layer has to come out
// as behind end to end.
func TestStackBehindTrunkRendersAsBehind(t *testing.T) {
	gql := stubGQL{routes: map[string]string{
		"query StackByPR": `{
			"repository": {
				"pullRequest": {
					"number": 101,
					"headRefName": "auth-layer",
					"state": "OPEN",
					"stack": {
						"number": 6,
						"size": 2,
						"baseRefName": "main",
						"entries": {
							"pageInfo": {"hasNextPage": false},
							"nodes": [
								{"position": 1, "pullRequest": {"number": 101, "state": "OPEN", "headRefName": "auth-layer", "baseRefName": "main", "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN"}},
								{"position": 2, "pullRequest": {"number": 102, "state": "OPEN", "headRefName": "api-endpoints", "baseRefName": "auth-layer", "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN"}}
							]
						}
					}
				}
			}
		}`,
		"query Comparisons": `{
			"repository": {
				"c0": {"compare": {"status": "BEHIND", "aheadBy": 0, "behindBy": 12}},
				"c1": {"compare": {"status": "AHEAD", "aheadBy": 4, "behindBy": 0}}
			}
		}`,
	}}
	client := github.NewTestClient(gql, stubREST{})
	stack, err := resolve.Load(client, resolve.Repo{Owner: "o", Name: "r"}, "auth-layer", "https://github.com/o/r/pull/101")
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := render.Write(&buf, stack, render.PlainPalette{}, render.Options{}); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		switch {
		case strings.Contains(line, "auth-layer") && !strings.Contains(line, "behind"):
			t.Fatalf("layer behind main should read behind:\n%s", buf.String())
		case strings.Contains(line, "api-endpoints") && !strings.Contains(line, "up to date"):
			t.Fatalf("layer current with the branch below should read up to date:\n%s", buf.String())
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
