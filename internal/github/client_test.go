package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeGQL struct {
	stackJSON string
	err       error
	lastQuery string
}

func (f *fakeGQL) Do(query string, variables map[string]interface{}, response interface{}) error {
	f.lastQuery = query
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.stackJSON), response)
}

type fakeREST struct {
	status int
	body   string
}

func (f *fakeREST) Request(method, path string, body io.Reader) (*http.Response, error) {
	code := f.status
	if code == 0 {
		code = 200
	}
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Header:     make(http.Header),
	}, nil
}

func TestLoadStackByPRGraphQL(t *testing.T) {
	gql := &fakeGQL{stackJSON: `{
		"repository": {
			"pullRequest": {
				"number": 102,
				"headRefName": "api-endpoints",
				"state": "OPEN",
				"stack": {
					"number": 6,
					"size": 2,
					"baseRefName": "main",
					"entries": {
						"pageInfo": {"hasNextPage": false},
						"nodes": [
							{"position": 1, "pullRequest": {"number": 101, "title": "auth", "url": "u1", "state": "OPEN", "headRefName": "auth-layer"}},
							{"position": 2, "pullRequest": {"number": 102, "title": "api", "url": "u2", "state": "OPEN", "headRefName": "api-endpoints"}}
						]
					}
				}
			}
		}
	}`}
	c := NewTestClient(gql, &fakeREST{status: 404, body: "[]"})
	stack, err := c.LoadStackByPR("o", "r", 102)
	if err != nil {
		t.Fatal(err)
	}
	if stack.Number != 6 || len(stack.PullRequests) != 2 {
		t.Fatalf("%+v", stack)
	}
	if stack.PullRequests[0].Branch != "auth-layer" {
		t.Fatalf("bottom %s", stack.PullRequests[0].Branch)
	}
}

func TestLoadStackByPRFallsBackToREST(t *testing.T) {
	gql := &fakeGQL{stackJSON: `{
		"repository": {
			"pullRequest": {
				"number": 101,
				"headRefName": "auth-layer",
				"state": "OPEN",
				"stack": null
			}
		}
	}`}
	restBody := `[{
		"number": 6,
		"base": {"ref": "main"},
		"pull_requests": [
			{"number": 101, "state": "open", "draft": false, "head": {"ref": "auth-layer", "sha": "abc"}},
			{"number": 102, "state": "open", "draft": false, "head": {"ref": "api-endpoints", "sha": "def"}}
		]
	}]`
	// After REST, hydrate calls GraphQL again with the multi-PR query.
	g := &seqGQL{responses: []string{
		gql.stackJSON,
		`{"repository": {
			"n0": {"number": 101, "title": "auth", "url": "u1", "state": "OPEN", "headRefName": "auth-layer"},
			"n1": {"number": 102, "title": "api", "url": "u2", "state": "OPEN", "headRefName": "api-endpoints"}
		}}`,
	}}
	c := NewTestClient(g, &fakeREST{status: 200, body: restBody})
	stack, err := c.LoadStackByPR("o", "r", 101)
	if err != nil {
		t.Fatal(err)
	}
	if stack.Number != 6 || stack.Base != "main" || len(stack.PullRequests) != 2 {
		t.Fatalf("%+v", stack)
	}
}

func TestLoadStackByPRNotInStack(t *testing.T) {
	gql := &fakeGQL{stackJSON: `{
		"repository": {
			"pullRequest": {
				"number": 9,
				"headRefName": "solo",
				"state": "OPEN",
				"stack": null
			}
		}
	}`}
	c := NewTestClient(gql, &fakeREST{status: 200, body: `[]`})
	_, err := c.LoadStackByPR("o", "r", 9)
	if err == nil || !strings.Contains(err.Error(), "not part of a GitHub stack") {
		t.Fatalf("got %v", err)
	}
}

func TestPRNumberForBranchPrefersOpen(t *testing.T) {
	gql := &fakeGQL{stackJSON: `{
		"repository": {
			"ref": {
				"associatedPullRequests": {
					"nodes": [
						{"number": 1, "state": "MERGED", "headRefName": "feat"},
						{"number": 4, "state": "OPEN", "headRefName": "feat"}
					]
				}
			}
		}
	}`}
	c := NewTestClient(gql, &fakeREST{})
	n, err := c.PRNumberForBranch("o", "r", "feat")
	if err != nil || n != 4 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

type seqGQL struct {
	responses []string
	i         int
}

func (s *seqGQL) Do(query string, variables map[string]interface{}, response interface{}) error {
	if s.i >= len(s.responses) {
		return fmt.Errorf("unexpected extra GraphQL call: %s", query)
	}
	raw := s.responses[s.i]
	s.i++
	return json.Unmarshal([]byte(raw), response)
}

func TestLoadStackByNumber(t *testing.T) {
	rest := &fakeREST{status: 200, body: `{
		"number": 6,
		"base": {"ref": "main"},
		"pull_requests": [
			{"number": 101, "state": "open", "head": {"ref": "auth-layer", "sha": "a"}}
		]
	}`}
	g := &seqGQL{responses: []string{
		`{"repository": {"pullRequest": {
			"number": 101,
			"headRefName": "auth-layer",
			"state": "OPEN",
			"stack": {
				"number": 6,
				"baseRefName": "main",
				"entries": {
					"pageInfo": {},
					"nodes": [
						{"position": 1, "pullRequest": {"number": 101, "title": "auth", "state": "OPEN", "headRefName": "auth-layer"}}
					]
				}
			}
		}}}`,
	}}
	c := NewTestClient(g, rest)
	stack, err := c.LoadStackByNumber("o", "r", 6)
	if err != nil {
		t.Fatal(err)
	}
	if stack.Number != 6 || stack.PullRequests[0].Number != 101 {
		t.Fatalf("%+v", stack)
	}
}

func TestRESTStackNotFound(t *testing.T) {
	c := NewTestClient(&fakeGQL{}, &fakeREST{status: 404, body: `{"message":"Not Found"}`})
	_, err := c.LoadStackByNumber("o", "r", 99)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFakeRESTReadsBody(_ *testing.T) {
	r := &fakeREST{body: "hi"}
	resp, _ := r.Request("GET", "/", bytes.NewReader(nil))
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "hi" {
		panic(string(b))
	}
}
