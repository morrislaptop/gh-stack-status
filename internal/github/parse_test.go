package github

import (
	"encoding/json"
	"testing"

	"github.com/morrislaptop/gh-stack-status/internal/model"
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
	c := rollupWith([]gqlContext{
		{Name: "build", Status: "COMPLETED", Conclusion: &ok},
		{Context: "coverage", State: "SUCCESS"},
	}, "SUCCESS")
	if c.Passed != 2 {
		t.Fatalf("passed %d", c.Passed)
	}
}

func rollupWith(nodes []gqlContext, state string) model.Checks {
	r := &gqlRollup{State: state}
	r.Contexts.Nodes = nodes
	return parseChecks(r)
}

// A check that was cancelled because a later run of the same check in the same
// workflow superseded it must not be reported as failing.
func TestSupersededCancelledRunIsNotFailing(t *testing.T) {
	cancelled := "CANCELLED"
	success := "SUCCESS"
	autoApprove := checkSuiteFor("Auto approve")
	codeStyle := checkSuiteFor("Code style")

	c := rollupWith([]gqlContext{
		{Typename: "CheckRun", Name: "build", Status: "COMPLETED", Conclusion: &cancelled, StartedAt: "2026-08-26T02:09:05Z", CheckSuite: autoApprove},
		{Typename: "CheckRun", Name: "build", Status: "COMPLETED", Conclusion: &success, StartedAt: "2026-08-26T02:09:10Z", CheckSuite: autoApprove},
		{Typename: "CheckRun", Name: "code-style", Status: "COMPLETED", Conclusion: &cancelled, StartedAt: "2026-08-26T02:09:08Z", CheckSuite: codeStyle},
		{Typename: "CheckRun", Name: "code-style", Status: "COMPLETED", Conclusion: &success, StartedAt: "2026-08-26T02:09:12Z", CheckSuite: codeStyle},
	}, "FAILURE")

	if c.State != "SUCCESS" {
		t.Fatalf("state %q, want SUCCESS (reported rollup state counts superseded runs)", c.State)
	}
	if c.Failed != 0 || len(c.FailedNames) != 0 {
		t.Fatalf("failed=%d names=%v", c.Failed, c.FailedNames)
	}
	if c.Passed != 2 {
		t.Fatalf("passed %d, want 2", c.Passed)
	}
}

// The same check name in different workflows is a different check.
func TestSameNameInDifferentWorkflowsKeptSeparate(t *testing.T) {
	success := "SUCCESS"
	fail := "FAILURE"
	c := rollupWith([]gqlContext{
		{Typename: "CheckRun", Name: "build", Status: "COMPLETED", Conclusion: &success, StartedAt: "2026-08-26T02:09:10Z", CheckSuite: checkSuiteFor("Auto approve")},
		{Typename: "CheckRun", Name: "build", Status: "COMPLETED", Conclusion: &fail, StartedAt: "2026-08-26T02:09:12Z", CheckSuite: checkSuiteFor("superquote-app checks")},
	}, "FAILURE")

	if c.Passed != 1 || c.Failed != 1 {
		t.Fatalf("passed=%d failed=%d", c.Passed, c.Failed)
	}
	if c.State != "FAILURE" {
		t.Fatalf("state %q", c.State)
	}
}

func TestNeutralAndSkippedCountAsSkipped(t *testing.T) {
	neutral := "NEUTRAL"
	skipped := "SKIPPED"
	success := "SUCCESS"
	c := rollupWith([]gqlContext{
		{Typename: "CheckRun", Name: "Header rules", Status: "COMPLETED", Conclusion: &neutral},
		{Typename: "CheckRun", Name: "Pages changed", Status: "COMPLETED", Conclusion: &skipped},
		{Typename: "CheckRun", Name: "build", Status: "COMPLETED", Conclusion: &success},
	}, "SUCCESS")

	if c.Skipped != 2 || c.Passed != 1 {
		t.Fatalf("skipped=%d passed=%d", c.Skipped, c.Passed)
	}
	if c.Counted() != 1 || c.Total() != 3 {
		t.Fatalf("counted=%d total=%d", c.Counted(), c.Total())
	}
	if c.State != "SUCCESS" {
		t.Fatalf("state %q", c.State)
	}
}

func TestStatusContextsDedupedByContext(t *testing.T) {
	c := rollupWith([]gqlContext{
		{Typename: "StatusContext", Context: "deploy/netlify", State: "FAILURE", CreatedAt: "2026-08-26T02:09:00Z"},
		{Typename: "StatusContext", Context: "deploy/netlify", State: "SUCCESS", CreatedAt: "2026-08-26T02:11:00Z"},
	}, "FAILURE")
	if c.Passed != 1 || c.Failed != 0 {
		t.Fatalf("passed=%d failed=%d", c.Passed, c.Failed)
	}
}

func TestPendingRunWinsOverOlderFailure(t *testing.T) {
	fail := "FAILURE"
	c := rollupWith([]gqlContext{
		{Typename: "CheckRun", Name: "test", Status: "COMPLETED", Conclusion: &fail, StartedAt: "2026-08-26T02:09:00Z", CheckSuite: checkSuiteFor("CI")},
		{Typename: "CheckRun", Name: "test", Status: "IN_PROGRESS", StartedAt: "2026-08-26T02:20:00Z", CheckSuite: checkSuiteFor("CI")},
	}, "FAILURE")
	if c.Pending != 1 || c.Failed != 0 || c.State != "PENDING" {
		t.Fatalf("%+v", c)
	}
}

func checkSuiteFor(workflow string) *struct {
	WorkflowRun *struct {
		Workflow *struct {
			Name string `json:"name"`
		} `json:"workflow"`
	} `json:"workflowRun"`
} {
	var ctx gqlContext
	if err := json.Unmarshal([]byte(`{"checkSuite":{"workflowRun":{"workflow":{"name":`+jsonString(workflow)+`}}}}`), &ctx); err != nil {
		panic(err)
	}
	return ctx.CheckSuite
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestParseRebaseWithoutComparison(t *testing.T) {
	cases := []struct {
		mergeable, state, want string
	}{
		{"MERGEABLE", "CLEAN", "UP_TO_DATE"},
		{"MERGEABLE", "UNSTABLE", "UP_TO_DATE"},
		{"MERGEABLE", "BLOCKED", "UP_TO_DATE"},
		{"MERGEABLE", "DRAFT", "UP_TO_DATE"},
		{"MERGEABLE", "BEHIND", "BEHIND"},
		{"CONFLICTING", "DIRTY", "CONFLICT"},
		{"CONFLICTING", "BLOCKED", "CONFLICT"},
		{"MERGEABLE", "DIRTY", "CONFLICT"},
		{"UNKNOWN", "UNKNOWN", "UNKNOWN"},
		{"", "", "UNKNOWN"},
	}
	for _, tc := range cases {
		got := parseRebase(tc.mergeable, tc.state, nil)
		if got.Status != tc.want {
			t.Errorf("parseRebase(%q, %q, nil) = %q, want %q", tc.mergeable, tc.state, got.Status, tc.want)
		}
	}
}

// The commit comparison decides being behind. mergeStateStatus reports BEHIND
// only where the base branch requires branches to be up to date, and DIRTY or
// BLOCKED mask it, so a stack GitHub offers to rebase usually reports CLEAN.
func TestParseRebaseWithComparison(t *testing.T) {
	cases := []struct {
		name             string
		mergeable, state string
		cmp              model.Comparison
		want             string
	}{
		{"behind while merge state says clean", "MERGEABLE", "CLEAN", model.Comparison{Status: "BEHIND", BehindBy: 4}, "BEHIND"},
		{"behind while blocked masks it", "MERGEABLE", "BLOCKED", model.Comparison{Status: "DIVERGED", AheadBy: 2, BehindBy: 9}, "BEHIND"},
		{"behind while checks fail", "MERGEABLE", "UNSTABLE", model.Comparison{Status: "DIVERGED", AheadBy: 1, BehindBy: 1}, "BEHIND"},
		{"conflict outranks behind", "CONFLICTING", "DIRTY", model.Comparison{Status: "DIVERGED", AheadBy: 3, BehindBy: 3}, "CONFLICT"},
		{"up to date", "MERGEABLE", "CLEAN", model.Comparison{Status: "AHEAD", AheadBy: 5}, "UP_TO_DATE"},
		{"identical", "MERGEABLE", "CLEAN", model.Comparison{Status: "IDENTICAL"}, "UP_TO_DATE"},
		// A head branch that already contains the tip of its base cannot
		// conflict, so it is up to date even before GitHub computes mergeable.
		{"up to date before mergeable is computed", "UNKNOWN", "UNKNOWN", model.Comparison{Status: "AHEAD", AheadBy: 2}, "UP_TO_DATE"},
		{"behind before mergeable is computed", "UNKNOWN", "UNKNOWN", model.Comparison{Status: "BEHIND", BehindBy: 7}, "BEHIND"},
	}
	for _, tc := range cases {
		cmp := tc.cmp
		got := parseRebase(tc.mergeable, tc.state, &cmp)
		if got.Status != tc.want {
			t.Errorf("%s: parseRebase(%q, %q, %+v) = %q, want %q", tc.name, tc.mergeable, tc.state, cmp, got.Status, tc.want)
		}
		if got.Comparison == nil || got.Comparison.BehindBy != tc.cmp.BehindBy {
			t.Errorf("%s: comparison not carried through: %+v", tc.name, got.Comparison)
		}
	}
}
