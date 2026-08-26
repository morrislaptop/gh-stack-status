package github

import (
	"encoding/json"
	"testing"
)

func TestParseChecksAndReviews(t *testing.T) {
	fail := "FAILURE"
	success := "SUCCESS"
	decision := "CHANGES_REQUESTED"
	pr := gqlPR{
		Number:         102,
		Title:          "API",
		URL:            "https://github.com/o/r/pull/102",
		State:          "OPEN",
		HeadRefName:    "api-endpoints",
		ReviewDecision: &decision,
		StatusCheckRollup: &gqlRollup{
			State: "FAILURE",
		},
	}
	pr.StatusCheckRollup.Contexts.Nodes = []gqlContext{
		{Typename: "CheckRun", Name: "lint", Status: "COMPLETED", Conclusion: &fail},
		{Typename: "CheckRun", Name: "test", Status: "COMPLETED", Conclusion: &success},
		{Typename: "StatusContext", Context: "deploy", State: "PENDING"},
	}
	pr.LatestReviews.Nodes = []struct {
		Author *struct {
			Login string `json:"login"`
		} `json:"author"`
		State string `json:"state"`
	}{
		{Author: &struct {
			Login string `json:"login"`
		}{Login: "bob"}, State: "CHANGES_REQUESTED"},
		{Author: &struct {
			Login string `json:"login"`
		}{Login: "alice"}, State: "APPROVED"},
	}
	pr.ReviewRequests.Nodes = []struct {
		RequestedReviewer *gqlReviewer `json:"requestedReviewer"`
	}{
		{RequestedReviewer: &gqlReviewer{Typename: "User", Login: "carol"}},
		{RequestedReviewer: &gqlReviewer{Typename: "User", Login: "bob"}},
	}

	got := pr.toModel(2)
	if got.Checks.State != "FAILURE" || got.Checks.Failed != 1 || got.Checks.Passed != 1 || got.Checks.Pending != 1 {
		t.Fatalf("checks: %+v", got.Checks)
	}
	if len(got.Checks.FailedNames) != 1 || got.Checks.FailedNames[0] != "lint" {
		t.Fatalf("failed names: %v", got.Checks.FailedNames)
	}
	if got.Reviews.Decision != "CHANGES_REQUESTED" {
		t.Fatalf("decision %q", got.Reviews.Decision)
	}
	if len(got.Reviews.Approved) != 1 || got.Reviews.Approved[0] != "alice" {
		t.Fatalf("approved %v", got.Reviews.Approved)
	}
	if len(got.Reviews.ChangesRequested) != 1 || got.Reviews.ChangesRequested[0] != "bob" {
		t.Fatalf("changes %v", got.Reviews.ChangesRequested)
	}
	if len(got.Reviews.Pending) != 1 || got.Reviews.Pending[0] != "carol" {
		t.Fatalf("pending should exclude bob, got %v", got.Reviews.Pending)
	}
}

func TestParseGQLStackOrdersByPosition(t *testing.T) {
	raw := `{
		"number": 6,
		"size": 3,
		"baseRefName": "main",
		"entries": {
			"nodes": [
				{"position": 3, "pullRequest": {"number": 103, "headRefName": "frontend", "state": "OPEN"}},
				{"position": 1, "pullRequest": {"number": 101, "headRefName": "auth-layer", "state": "OPEN"}},
				{"position": 2, "pullRequest": {"number": 102, "headRefName": "api-endpoints", "state": "OPEN"}}
			]
		}
	}`
	var meta gqlStack
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatal(err)
	}
	stack := parseGQLStack(&meta, meta.Entries.Nodes)
	if stack.Number != 6 || stack.Base != "main" {
		t.Fatalf("meta %+v", stack)
	}
	if len(stack.PullRequests) != 3 {
		t.Fatalf("len %d", len(stack.PullRequests))
	}
	if stack.PullRequests[0].Number != 101 || stack.PullRequests[1].Number != 102 || stack.PullRequests[2].Number != 103 {
		t.Fatalf("order %+v", stack.PullRequests)
	}
}

func TestParseChecksEmpty(t *testing.T) {
	c := parseChecks(nil)
	if c.State != "" || c.Total() != 0 {
		t.Fatalf("%+v", c)
	}
}

func TestParseChecksInfersTypename(t *testing.T) {
	ok := "SUCCESS"
	c := parseChecks(&gqlRollup{
		State: "SUCCESS",
		Contexts: struct {
			Nodes []gqlContext `json:"nodes"`
		}{Nodes: []gqlContext{
			{Name: "build", Status: "COMPLETED", Conclusion: &ok},
			{Context: "coverage", State: "SUCCESS"},
		}},
	})
	if c.Passed != 2 {
		t.Fatalf("passed %d", c.Passed)
	}
}
