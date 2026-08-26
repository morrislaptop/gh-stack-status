package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/morrislaptop/gh-stack-status/internal/model"
)

const apiVersion = "2026-03-10"

type gqlClient interface {
	Do(query string, variables map[string]interface{}, response interface{}) error
}

type restClient interface {
	Request(method string, path string, body io.Reader) (*http.Response, error)
}

type Client struct {
	gql  gqlClient
	rest restClient
}

func NewClient() (*Client, error) {
	gql, err := api.DefaultGraphQLClient()
	if err != nil {
		return nil, err
	}
	rest, err := api.NewRESTClient(api.ClientOptions{
		Headers: map[string]string{
			"Accept":               "application/vnd.github+json",
			"X-GitHub-Api-Version": apiVersion,
		},
	})
	if err != nil {
		return nil, err
	}
	return &Client{gql: gql, rest: rest}, nil
}

func NewTestClient(gql gqlClient, rest restClient) *Client {
	return &Client{gql: gql, rest: rest}
}

func (c *Client) LoadStackByPR(owner, repo string, number int) (*model.Stack, error) {
	stack, pr, err := c.queryStackByPR(owner, repo, number)
	if err == nil && stack != nil && len(stack.PullRequests) > 0 {
		return stack, nil
	}
	graphqlErr := err

	restStack, restErr := c.restStackByPullRequest(owner, repo, number)
	if restErr == nil && restStack != nil {
		return c.hydrateRESTStack(owner, repo, restStack)
	}

	if graphqlErr != nil {
		return nil, graphqlErr
	}
	if restErr != nil {
		return nil, restErr
	}
	if pr == nil {
		return nil, fmt.Errorf("pull request #%d not found in %s/%s", number, owner, repo)
	}
	return nil, fmt.Errorf("pull request #%d is not part of a GitHub stack", number)
}

func (c *Client) LoadStackByNumber(owner, repo string, stackNumber int) (*model.Stack, error) {
	restStack, err := c.restStackByNumber(owner, repo, stackNumber)
	if err != nil {
		return nil, err
	}
	if restStack == nil || len(restStack.PullRequests) == 0 {
		return nil, fmt.Errorf("stack #%d has no pull requests", stackNumber)
	}
	stack, _, gqlErr := c.queryStackByPR(owner, repo, restStack.PullRequests[0].Number)
	if gqlErr == nil && stack != nil && len(stack.PullRequests) > 0 {
		return stack, nil
	}
	return c.hydrateRESTStack(owner, repo, restStack)
}

func (c *Client) PRNumberForBranch(owner, repo, branch string) (int, error) {
	var data struct {
		Repository *struct {
			Ref *struct {
				AssociatedPullRequests struct {
					Nodes []struct {
						Number      int    `json:"number"`
						State       string `json:"state"`
						HeadRefName string `json:"headRefName"`
					} `json:"nodes"`
				} `json:"associatedPullRequests"`
			} `json:"ref"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{
		"owner":          owner,
		"name":           repo,
		"qualifiedName":  "refs/heads/" + strings.TrimPrefix(branch, "refs/heads/"),
	}
	if err := c.gql.Do(branchPRQuery, vars, &data); err != nil {
		return 0, fmt.Errorf("looking up pull request for branch %q: %w", branch, err)
	}
	if data.Repository == nil || data.Repository.Ref == nil {
		return 0, fmt.Errorf("no pull request found for branch %q", branch)
	}
	var open, anyPR *int
	for i, n := range data.Repository.Ref.AssociatedPullRequests.Nodes {
		if n.HeadRefName != "" && n.HeadRefName != branch {
			continue
		}
		num := n.Number
		if anyPR == nil {
			anyPR = &num
		}
		if strings.EqualFold(n.State, "OPEN") && open == nil {
			open = &data.Repository.Ref.AssociatedPullRequests.Nodes[i].Number
		}
	}
	if open != nil {
		return *open, nil
	}
	if anyPR != nil {
		return *anyPR, nil
	}
	return 0, fmt.Errorf("no pull request found for branch %q", branch)
}

func (c *Client) queryStackByPR(owner, repo string, number int) (*model.Stack, *gqlPR, error) {
	var first *gqlPR
	var entries []gqlStackEntry
	var meta *gqlStack
	var cursor *string

	for {
		var data gqlStackByPRData
		vars := map[string]interface{}{
			"owner":  owner,
			"name":   repo,
			"number": number,
		}
		if cursor != nil {
			vars["cursor"] = *cursor
		} else {
			vars["cursor"] = nil
		}
		if err := c.gql.Do(stackByPRQuery, vars, &data); err != nil {
			return nil, nil, err
		}
		if data.Repository == nil || data.Repository.PullRequest == nil {
			return nil, nil, nil
		}
		pr := data.Repository.PullRequest
		if first == nil {
			first = &pr.gqlPR
		}
		if pr.Stack == nil {
			break
		}
		if meta == nil {
			copy := pr.Stack
			meta = copy
		}
		entries = append(entries, pr.Stack.Entries.Nodes...)
		if !pr.Stack.Entries.PageInfo.HasNextPage || pr.Stack.Entries.PageInfo.EndCursor == "" {
			break
		}
		c := pr.Stack.Entries.PageInfo.EndCursor
		cursor = &c
	}

	if meta == nil {
		return nil, first, nil
	}
	for _, e := range entries {
		if err := c.fetchRemainingContexts(owner, repo, e.PullRequest); err != nil {
			return nil, nil, err
		}
	}
	stack := parseGQLStack(meta, entries)
	return stack, first, nil
}

// fetchRemainingContexts pages through check contexts beyond the first page.
// Without every context, a superseded run can be mistaken for the latest one.
func (c *Client) fetchRemainingContexts(owner, repo string, pr *gqlPR) error {
	if pr == nil || pr.StatusCheckRollup == nil {
		return nil
	}
	page := pr.StatusCheckRollup.Contexts.PageInfo
	for page.HasNextPage && page.EndCursor != "" {
		var data struct {
			Repository *struct {
				PullRequest *struct {
					StatusCheckRollup *struct {
						Contexts gqlContexts `json:"contexts"`
					} `json:"statusCheckRollup"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		vars := map[string]interface{}{
			"owner":  owner,
			"name":   repo,
			"number": pr.Number,
			"cursor": page.EndCursor,
		}
		if err := c.gql.Do(prContextsQuery, vars, &data); err != nil {
			return err
		}
		if data.Repository == nil || data.Repository.PullRequest == nil ||
			data.Repository.PullRequest.StatusCheckRollup == nil {
			return nil
		}
		next := data.Repository.PullRequest.StatusCheckRollup.Contexts
		pr.StatusCheckRollup.Contexts.Nodes = append(pr.StatusCheckRollup.Contexts.Nodes, next.Nodes...)
		if next.PageInfo == page {
			return nil
		}
		page = next.PageInfo
	}
	return nil
}

func (c *Client) hydrateRESTStack(owner, repo string, rest *restStack) (*model.Stack, error) {
	numbers := make([]int, 0, len(rest.PullRequests))
	for _, pr := range rest.PullRequests {
		numbers = append(numbers, pr.Number)
	}
	prs, err := c.queryPRsByNumbers(owner, repo, numbers)
	if err != nil {
		return nil, err
	}
	byNum := map[int]model.PullRequest{}
	for _, pr := range prs {
		byNum[pr.Number] = pr
	}
	out := &model.Stack{
		Number: rest.Number,
		Base:   rest.Base.Ref,
	}
	for i, restPR := range rest.PullRequests {
		pr, ok := byNum[restPR.Number]
		if !ok {
			pr = model.PullRequest{
				Number: restPR.Number,
				Branch: restPR.Head.Ref,
				State:  strings.ToUpper(restPR.State),
				Draft:  restPR.Draft,
			}
		}
		pr.Position = i + 1
		out.PullRequests = append(out.PullRequests, pr)
	}
	return out, nil
}

func (c *Client) queryPRsByNumbers(owner, repo string, numbers []int) ([]model.PullRequest, error) {
	if len(numbers) == 0 {
		return nil, nil
	}
	var b strings.Builder
	b.WriteString(prsByNumbersPrefix)
	vars := map[string]interface{}{
		"owner": owner,
		"name":  repo,
	}
	for i, n := range numbers {
		alias := fmt.Sprintf("n%d", i)
		fmt.Fprintf(&b, ", $%s: Int!", alias)
		vars[alias] = n
	}
	b.WriteString(") {\n  repository(owner: $owner, name: $name) {\n")
	for i := range numbers {
		alias := fmt.Sprintf("n%d", i)
		fmt.Fprintf(&b, "    %s: pullRequest(number: $%s) { ...PRStatus }\n", alias, alias)
	}
	b.WriteString("  }\n}\n")

	var wrap struct {
		Repository map[string]*gqlPR `json:"repository"`
	}
	if err := c.gql.Do(b.String(), vars, &wrap); err != nil {
		return nil, err
	}
	var out []model.PullRequest
	if wrap.Repository == nil {
		return out, nil
	}
	for i := range numbers {
		alias := fmt.Sprintf("n%d", i)
		if pr, ok := wrap.Repository[alias]; ok && pr != nil {
			if err := c.fetchRemainingContexts(owner, repo, pr); err != nil {
				return nil, err
			}
			out = append(out, pr.toModel(i+1))
		}
	}
	return out, nil
}

func (c *Client) restStackByPullRequest(owner, repo string, pr int) (*restStack, error) {
	path := fmt.Sprintf("repos/%s/%s/stacks?pull_request=%d",
		url.PathEscape(owner), url.PathEscape(repo), pr)
	return c.getRESTStackListFirst(path)
}

func (c *Client) restStackByNumber(owner, repo string, number int) (*restStack, error) {
	path := fmt.Sprintf("repos/%s/%s/stacks/%d",
		url.PathEscape(owner), url.PathEscape(repo), number)
	resp, err := c.rest.Request("GET", path, nil)
	if err != nil {
		if status, ok := httpStatus(err); ok && status == http.StatusNotFound {
			return nil, fmt.Errorf("stack #%d not found", number)
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("stack #%d not found", number)
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var s restStack
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) getRESTStackListFirst(path string) (*restStack, error) {
	resp, err := c.rest.Request("GET", path, nil)
	if err != nil {
		if status, ok := httpStatus(err); ok && status == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var stacks []restStack
	if err := json.NewDecoder(resp.Body).Decode(&stacks); err != nil {
		return nil, err
	}
	if len(stacks) == 0 {
		return nil, nil
	}
	return &stacks[0], nil
}

func httpStatus(err error) (int, bool) {
	if he, ok := err.(*api.HTTPError); ok {
		return he.StatusCode, true
	}
	if err != nil && strings.Contains(err.Error(), "404") {
		return 404, true
	}
	return 0, false
}
