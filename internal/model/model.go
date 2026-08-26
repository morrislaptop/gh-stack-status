package model

import "strings"

// Stack is an ordered GitHub pull request stack (bottom closest to trunk).
type Stack struct {
	Number         int
	Base           string
	CurrentBranch  string
	PullRequests   []PullRequest // bottom (position 1) to top
}

type PullRequest struct {
	Number   int
	Title    string
	URL      string
	Branch   string
	State    string
	Draft    bool
	Position int
	Checks   Checks
	Reviews  Reviews
}

type Checks struct {
	State       string // SUCCESS, FAILURE, PENDING, ERROR, EXPECTED, or empty
	Passed      int
	Failed      int
	Pending     int
	FailedNames []string
}

func (c Checks) Total() int {
	return c.Passed + c.Failed + c.Pending
}

type Reviews struct {
	Decision          string // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, or empty
	Approved          []string
	ChangesRequested  []string
	Pending           []string
}

func (s Stack) CurrentIndex() int {
	if s.CurrentBranch == "" {
		return -1
	}
	for i, pr := range s.PullRequests {
		if pr.Branch == s.CurrentBranch {
			return i
		}
	}
	return -1
}

func (pr PullRequest) IsCurrent(currentBranch string) bool {
	return currentBranch != "" && pr.Branch == currentBranch
}

func NormalizeDecision(d string) string {
	return strings.ToUpper(strings.TrimSpace(d))
}
