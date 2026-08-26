package github

import (
	"fmt"
	"sort"
	"strings"

	"github.com/morrislaptop/gh-stack-status/internal/model"
)

func parseGQLStack(meta *gqlStack, entries []gqlStackEntry) *model.Stack {
	byPos := map[int]model.PullRequest{}
	for _, e := range entries {
		if e.PullRequest == nil {
			continue
		}
		pos := e.Position
		byPos[pos] = e.PullRequest.toModel(pos)
	}
	positions := make([]int, 0, len(byPos))
	for p := range byPos {
		positions = append(positions, p)
	}
	sort.Ints(positions)
	stack := &model.Stack{
		Number: meta.Number,
		Base:   meta.BaseRefName,
	}
	for _, p := range positions {
		stack.PullRequests = append(stack.PullRequests, byPos[p])
	}
	return stack
}

func (pr *gqlPR) toModel(position int) model.PullRequest {
	out := model.PullRequest{
		Number:   pr.Number,
		Title:    pr.Title,
		URL:      pr.URL,
		Branch:   pr.HeadRefName,
		State:    pr.State,
		Draft:    pr.IsDraft,
		Position: position,
		Checks:   parseChecks(pr.StatusCheckRollup),
		Reviews:  parseReviews(pr.ReviewDecision, pr),
	}
	return out
}

func parseChecks(rollup *gqlRollup) model.Checks {
	if rollup == nil {
		return model.Checks{}
	}
	c := model.Checks{}
	seenFail := map[string]bool{}
	for _, ctx := range dedupeContexts(rollup.Contexts.Nodes) {
		kind, name := classifyContext(ctx)
		switch kind {
		case "passed":
			c.Passed++
		case "skipped":
			c.Skipped++
		case "failed":
			c.Failed++
			if name != "" && !seenFail[name] {
				seenFail[name] = true
				c.FailedNames = append(c.FailedNames, name)
			}
		case "pending":
			c.Pending++
		}
	}
	c.State = rollupState(c, rollup.State)
	return c
}

// rollupState derives the overall state from the deduplicated contexts.
//
// statusCheckRollup.state counts every raw check run, including runs that were
// superseded by a later run of the same check, so it reports FAILURE for checks
// that GitHub itself displays as passing.
func rollupState(c model.Checks, reported string) string {
	if c.Total() == 0 {
		return strings.ToUpper(reported)
	}
	switch {
	case c.Failed > 0:
		return "FAILURE"
	case c.Pending > 0:
		return "PENDING"
	default:
		return "SUCCESS"
	}
}

// dedupeContexts keeps only the most recent run of each check, matching how the
// GitHub CLI and web UI collapse repeated runs. Check runs are identified by
// name and workflow; commit statuses by context.
func dedupeContexts(nodes []gqlContext) []gqlContext {
	ordered := make([]gqlContext, len(nodes))
	copy(ordered, nodes)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].startedTime() > ordered[j].startedTime()
	})

	seen := map[string]bool{}
	unique := make([]gqlContext, 0, len(ordered))
	for _, ctx := range ordered {
		var key string
		if normalizeTypename(ctx) == "StatusContext" {
			key = "status:" + ctx.Context
		} else {
			key = fmt.Sprintf("check:%s/%s", ctx.Name, ctx.workflowName())
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, ctx)
	}
	return unique
}

func normalizeTypename(ctx gqlContext) string {
	if ctx.Typename != "" {
		return ctx.Typename
	}
	if ctx.Name != "" || ctx.Conclusion != nil || ctx.Status != "" {
		return "CheckRun"
	}
	if ctx.Context != "" {
		return "StatusContext"
	}
	return ""
}

func classifyContext(ctx gqlContext) (kind, name string) {
	switch normalizeTypename(ctx) {
	case "CheckRun":
		name = ctx.Name
		status := strings.ToUpper(ctx.Status)
		if status != "" && status != "COMPLETED" {
			return "pending", name
		}
		if ctx.Conclusion == nil {
			return "pending", name
		}
		switch strings.ToUpper(*ctx.Conclusion) {
		case "SUCCESS":
			return "passed", name
		case "NEUTRAL", "SKIPPED":
			return "skipped", name
		default:
			// FAILURE, CANCELLED, TIMED_OUT, ACTION_REQUIRED, STALE, STARTUP_FAILURE
			return "failed", name
		}
	case "StatusContext":
		name = ctx.Context
		switch strings.ToUpper(ctx.State) {
		case "SUCCESS":
			return "passed", name
		case "NEUTRAL", "SKIPPED":
			return "skipped", name
		case "FAILURE", "ERROR":
			return "failed", name
		default:
			return "pending", name
		}
	default:
		return "", ""
	}
}

func parseReviews(decision *string, pr *gqlPR) model.Reviews {
	r := model.Reviews{}
	if decision != nil {
		r.Decision = strings.ToUpper(*decision)
	}
	approved := map[string]bool{}
	changes := map[string]bool{}
	for _, n := range pr.LatestReviews.Nodes {
		if n.Author == nil || n.Author.Login == "" {
			continue
		}
		login := n.Author.Login
		switch strings.ToUpper(n.State) {
		case "APPROVED":
			if !approved[login] {
				approved[login] = true
				r.Approved = append(r.Approved, login)
			}
		case "CHANGES_REQUESTED":
			if !changes[login] {
				changes[login] = true
				r.ChangesRequested = append(r.ChangesRequested, login)
			}
		}
	}
	pendingSeen := map[string]bool{}
	for _, n := range pr.ReviewRequests.Nodes {
		name := n.RequestedReviewer.DisplayName()
		if name == "" {
			continue
		}
		if approved[name] || changes[name] || pendingSeen[name] {
			continue
		}
		pendingSeen[name] = true
		r.Pending = append(r.Pending, name)
	}
	return r
}
