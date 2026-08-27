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

// routedGQL answers each query by matching a substring of it, so a test can
// serve the stack query and the comparison query without depending on call
// order.
type routedGQL struct {
	routes  map[string]string
	queries []string
}

func (r *routedGQL) Do(query string, variables map[string]interface{}, response interface{}) error {
	r.queries = append(r.queries, query)
	for marker, body := range r.routes {
		if strings.Contains(query, marker) {
			return json.Unmarshal([]byte(body), response)
		}
	}
	return fmt.Errorf("no route for query: %s", query)
}

// A stack that is behind its base reports CLEAN when the base branch does not
// require branches to be up to date, so the commit comparison has to be what
// decides it.
func TestLoadStackByPRUsesComparisonForBehind(t *testing.T) {
	gql := &routedGQL{routes: map[string]string{
		"query StackByPR": `{
			"repository": {
				"pullRequest": {
					"number": 102,
					"headRefName": "api-endpoints",
					"baseRefName": "auth-layer",
					"state": "OPEN",
					"stack": {
						"number": 6,
						"size": 2,
						"baseRefName": "main",
						"entries": {
							"pageInfo": {"hasNextPage": false},
							"nodes": [
								{"position": 1, "pullRequest": {"number": 101, "state": "OPEN", "headRefName": "auth-layer", "baseRefName": "main", "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN"}},
								{"position": 2, "pullRequest": {"number": 102, "state": "OPEN", "headRefName": "api-endpoints", "baseRefName": "auth-layer", "mergeable": "MERGEABLE", "mergeStateStatus": "BLOCKED"}}
							]
						}
					}
				}
			}
		}`,
		"query Comparisons": `{
			"repository": {
				"c0": {"compare": {"status": "BEHIND", "aheadBy": 0, "behindBy": 12}},
				"c1": {"compare": {"status": "AHEAD", "aheadBy": 3, "behindBy": 0}}
			}
		}`,
	}}
	c := NewTestClient(gql, &fakeREST{status: 404, body: "[]"})
	stack, err := c.LoadStackByPR("o", "r", 102)
	if err != nil {
		t.Fatal(err)
	}
	bottom, top := stack.PullRequests[0], stack.PullRequests[1]
	if bottom.Rebase.Status != "BEHIND" {
		t.Fatalf("#%d: got %q, want BEHIND (12 commits behind main)", bottom.Number, bottom.Rebase.Status)
	}
	if bottom.Rebase.Comparison == nil || bottom.Rebase.Comparison.BehindBy != 12 {
		t.Fatalf("#%d comparison %+v", bottom.Number, bottom.Rebase.Comparison)
	}
	if top.Rebase.Status != "UP_TO_DATE" {
		t.Fatalf("#%d: got %q, want UP_TO_DATE", top.Number, top.Rebase.Status)
	}
	if bottom.BaseBranch != "main" || top.BaseBranch != "auth-layer" {
		t.Fatalf("base branches %q %q", bottom.BaseBranch, top.BaseBranch)
	}
}

// The comparison is supplementary: when it cannot be fetched, the merge-state
// answer stands and the stack still renders.
func TestComparisonFailureLeavesMergeStateAnswer(t *testing.T) {
	gql := &routedGQL{routes: map[string]string{
		"query StackByPR": `{
			"repository": {
				"pullRequest": {
					"number": 101,
					"headRefName": "auth-layer",
					"state": "OPEN",
					"stack": {
						"number": 6,
						"baseRefName": "main",
						"entries": {
							"pageInfo": {},
							"nodes": [
								{"position": 1, "pullRequest": {"number": 101, "state": "OPEN", "headRefName": "auth-layer", "baseRefName": "main", "mergeable": "MERGEABLE", "mergeStateStatus": "BEHIND"}}
							]
						}
					}
				}
			}
		}`,
	}}
	c := NewTestClient(gql, &fakeREST{status: 404, body: "[]"})
	stack, err := c.LoadStackByPR("o", "r", 101)
	if err != nil {
		t.Fatal(err)
	}
	if got := stack.PullRequests[0].Rebase.Status; got != "BEHIND" {
		t.Fatalf("got %q, want BEHIND from mergeStateStatus", got)
	}
	if stack.PullRequests[0].Rebase.Comparison != nil {
		t.Fatalf("comparison should be absent: %+v", stack.PullRequests[0].Rebase.Comparison)
	}
}

// Closed and merged layers are not compared: their branches have moved on and a
// comparison would report drift that no rebase is expected to fix.
func TestComparisonSkipsClosedPullRequests(t *testing.T) {
	gql := &routedGQL{routes: map[string]string{
		"query Comparisons": `{"repository": {"c0": {"compare": {"status": "BEHIND", "behindBy": 4}}}}`,
	}}
	c := NewTestClient(gql, &fakeREST{})
	open := &gqlPR{Number: 1, State: "OPEN", HeadRefName: "feat", BaseRefName: "main"}
	merged := &gqlPR{Number: 2, State: "MERGED", HeadRefName: "old", BaseRefName: "main"}
	c.fillComparisons("o", "r", []*gqlPR{merged, open})

	if merged.Comparison != nil {
		t.Fatalf("merged PR compared: %+v", merged.Comparison)
	}
	if open.Comparison == nil || open.Comparison.BehindBy != 4 {
		t.Fatalf("open PR comparison %+v", open.Comparison)
	}
	if len(gql.queries) != 1 {
		t.Fatalf("queries %d, want 1", len(gql.queries))
	}
	want := "c0: ref(qualifiedName: $b0) { compare(headRef: $h0) { status aheadBy behindBy } }"
	if !strings.Contains(gql.queries[0], want) {
		t.Fatalf("query does not compare the base ref against the head ref:\n%s", gql.queries[0])
	}
}

func TestComparisonBatchesLargeStacks(t *testing.T) {
	gql := &routedGQL{routes: map[string]string{"query Comparisons": `{"repository": {}}`}}
	c := NewTestClient(gql, &fakeREST{})
	prs := make([]*gqlPR, 0, comparisonBatch+1)
	for i := 0; i <= comparisonBatch; i++ {
		prs = append(prs, &gqlPR{Number: i + 1, State: "OPEN", HeadRefName: fmt.Sprintf("layer-%d", i), BaseRefName: "main"})
	}
	c.fillComparisons("o", "r", prs)
	if len(gql.queries) != 2 {
		t.Fatalf("queries %d, want 2 batches for %d pull requests", len(gql.queries), len(prs))
	}
}

func TestCompareHeadRefQualifiesForks(t *testing.T) {
	same := &gqlPR{HeadRefName: "feat"}
	if got := same.compareHeadRef("o"); got != "feat" {
		t.Fatalf("same repo: %q", got)
	}
	fork := &gqlPR{HeadRefName: "feat", IsCrossRepository: true}
	fork.HeadRepositoryOwner = &struct {
		Login string `json:"login"`
	}{Login: "contributor"}
	if got := fork.compareHeadRef("o"); got != "contributor:feat" {
		t.Fatalf("fork: %q", got)
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
