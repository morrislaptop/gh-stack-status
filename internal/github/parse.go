package github

import (
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
	c := model.Checks{State: strings.ToUpper(rollup.State)}
	seenFail := map[string]bool{}
	for _, ctx := range rollup.Contexts.Nodes {
		kind, name := classifyContext(ctx)
		switch kind {
		case "passed":
			c.Passed++
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
	if c.State == "" && c.Total() > 0 {
		switch {
		case c.Failed > 0:
			c.State = "FAILURE"
		case c.Pending > 0:
			c.State = "PENDING"
		default:
			c.State = "SUCCESS"
		}
	}
	return c
}

func classifyContext(ctx gqlContext) (kind, name string) {
	switch ctx.Typename {
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
		case "SUCCESS", "NEUTRAL", "SKIPPED":
			return "passed", name
		case "FAILURE", "TIMED_OUT", "CANCELLED", "STARTUP_FAILURE", "ACTION_REQUIRED", "STALE":
			return "failed", name
		default:
			return "pending", name
		}
	case "StatusContext":
		name = ctx.Context
		switch strings.ToUpper(ctx.State) {
		case "SUCCESS":
			return "passed", name
		case "FAILURE", "ERROR":
			return "failed", name
		default:
			return "pending", name
		}
	default:
		// Some payloads omit __typename; infer from fields.
		if ctx.Name != "" || ctx.Conclusion != nil || ctx.Status != "" {
			ctx.Typename = "CheckRun"
			return classifyContext(ctx)
		}
		if ctx.Context != "" {
			ctx.Typename = "StatusContext"
			return classifyContext(ctx)
		}
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
	if r.Decision == "" && pr.IsDraft {
		r.Decision = ""
	}
	return r
}
