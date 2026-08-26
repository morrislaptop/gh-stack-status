package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/morrislaptop/gh-stack-status/internal/model"
)

type Palette interface {
	Green(string) string
	Red(string) string
	Yellow(string) string
	Gray(string) string
	Bold(string) string
	Cyan(string) string
}

type PlainPalette struct{}

func (PlainPalette) Green(s string) string  { return s }
func (PlainPalette) Red(s string) string    { return s }
func (PlainPalette) Yellow(s string) string { return s }
func (PlainPalette) Gray(s string) string   { return s }
func (PlainPalette) Bold(s string) string   { return s }
func (PlainPalette) Cyan(s string) string   { return s }

type Options struct {
	JSON  bool
	Short bool
}

func Write(w io.Writer, stack *model.Stack, pal Palette, opts Options) error {
	if pal == nil {
		pal = PlainPalette{}
	}
	if opts.JSON {
		return writeJSON(w, stack)
	}
	if opts.Short {
		return writeShort(w, stack, pal)
	}
	return writeTable(w, stack, pal)
}

type jsonStack struct {
	Number         int       `json:"number"`
	Base           string    `json:"base"`
	CurrentBranch  string    `json:"currentBranch"`
	PullRequests   []jsonPR  `json:"pullRequests"`
}

type jsonPR struct {
	Number    int         `json:"number"`
	Title     string      `json:"title"`
	URL       string      `json:"url"`
	Branch    string      `json:"branch"`
	State     string      `json:"state"`
	Draft     bool        `json:"draft"`
	IsCurrent bool        `json:"isCurrent"`
	Position  int         `json:"position"`
	Rebase    jsonRebase  `json:"rebase"`
	Checks    jsonChecks  `json:"checks"`
	Reviews   jsonReviews `json:"reviews"`
}

type jsonRebase struct {
	Status           string `json:"status"`
	Mergeable        string `json:"mergeable"`
	MergeStateStatus string `json:"mergeStateStatus"`
}

type jsonChecks struct {
	State       string   `json:"state"`
	Passed      int      `json:"passed"`
	Failed      int      `json:"failed"`
	Pending     int      `json:"pending"`
	Skipped     int      `json:"skipped"`
	FailedNames []string `json:"failedNames"`
}

type jsonReviews struct {
	Decision         string   `json:"decision"`
	Approved         []string `json:"approved"`
	ChangesRequested []string `json:"changesRequested"`
	Pending          []string `json:"pending"`
}

func writeJSON(w io.Writer, stack *model.Stack) error {
	out := jsonStack{
		Number:        stack.Number,
		Base:          stack.Base,
		CurrentBranch: stack.CurrentBranch,
	}
	for _, pr := range stack.PullRequests {
		failed := pr.Checks.FailedNames
		if failed == nil {
			failed = []string{}
		}
		out.PullRequests = append(out.PullRequests, jsonPR{
			Number:    pr.Number,
			Title:     pr.Title,
			URL:       pr.URL,
			Branch:    pr.Branch,
			State:     pr.State,
			Draft:     pr.Draft,
			IsCurrent: pr.IsCurrent(stack.CurrentBranch),
			Position:  pr.Position,
			Rebase: jsonRebase{
				Status:           pr.Rebase.Status,
				Mergeable:        pr.Rebase.Mergeable,
				MergeStateStatus: pr.Rebase.MergeStateStatus,
			},
			Checks: jsonChecks{
				State:       pr.Checks.State,
				Passed:      pr.Checks.Passed,
				Failed:      pr.Checks.Failed,
				Pending:     pr.Checks.Pending,
				Skipped:     pr.Checks.Skipped,
				FailedNames: failed,
			},
			Reviews: jsonReviews{
				Decision:         pr.Reviews.Decision,
				Approved:         nonNil(pr.Reviews.Approved),
				ChangesRequested: nonNil(pr.Reviews.ChangesRequested),
				Pending:          nonNil(pr.Reviews.Pending),
			},
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func writeShort(w io.Writer, stack *model.Stack, pal Palette) error {
	prs := displayOrder(stack.PullRequests)
	for _, pr := range prs {
		fmt.Fprintf(w, "#%d  %s  %s  %s\n", pr.Number, FormatRebase(pr.Rebase, pal), FormatChecks(pr.Checks, pal), FormatReviews(pr.Reviews, pal))
	}
	return nil
}

func writeTable(w io.Writer, stack *model.Stack, pal Palette) error {
	n := len(stack.PullRequests)
	prWord := "PRs"
	if n == 1 {
		prWord = "PR"
	}
	header := fmt.Sprintf("Stack #%d  targeting %s  %d %s", stack.Number, stack.Base, n, prWord)
	fmt.Fprintln(w, pal.Bold(header))
	fmt.Fprintln(w)

	prs := displayOrder(stack.PullRequests)
	branchW, rebaseW, checksW := 0, 0, 0
	type row struct {
		marker, branch, pr, rebase, checks, reviews string
	}
	rows := make([]row, 0, len(prs))
	for _, pr := range prs {
		marker := " "
		if pr.IsCurrent(stack.CurrentBranch) {
			marker = pal.Cyan("»")
		}
		rebase := FormatRebase(pr.Rebase, pal)
		checks := FormatChecks(pr.Checks, pal)
		reviews := FormatReviews(pr.Reviews, pal)
		if len(pr.Branch) > branchW {
			branchW = len(pr.Branch)
		}
		if visibleLen(rebase) > rebaseW {
			rebaseW = visibleLen(rebase)
		}
		if visibleLen(checks) > checksW {
			checksW = visibleLen(checks)
		}
		rows = append(rows, row{
			marker:  marker,
			branch:  pr.Branch,
			pr:      fmt.Sprintf("#%d", pr.Number),
			rebase:  rebase,
			checks:  checks,
			reviews: reviews,
		})
	}
	if branchW < 6 {
		branchW = 6
	}

	lineW := visibleLen(header)
	outLines := make([]string, 0, len(rows))
	for _, r := range rows {
		line := fmt.Sprintf("%s %-*s  %-5s  %s  %s  %s",
			padMarker(r.marker),
			branchW, r.branch,
			r.pr,
			padVisible(r.rebase, rebaseW),
			padVisible(r.checks, checksW),
			r.reviews,
		)
		outLines = append(outLines, line)
		if visibleLen(line) > lineW {
			lineW = visibleLen(line)
		}
	}
	for _, line := range outLines {
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w, pal.Gray(strings.Repeat("─", max(12, lineW))))
	fmt.Fprintf(w, "  %s\n", pal.Gray(stack.Base))
	return nil
}

func padMarker(m string) string {
	if m == " " || m == "" {
		return " "
	}
	return m
}

func displayOrder(prs []model.PullRequest) []model.PullRequest {
	out := make([]model.PullRequest, len(prs))
	for i, pr := range prs {
		out[len(prs)-1-i] = pr
	}
	return out
}

func FormatRebase(r model.Rebase, pal Palette) string {
	if pal == nil {
		pal = PlainPalette{}
	}
	switch strings.ToUpper(r.Status) {
	case "UP_TO_DATE":
		return pal.Green("up to date")
	case "BEHIND":
		return pal.Yellow("behind")
	case "CONFLICT":
		return pal.Red("conflict")
	default:
		return pal.Gray("—")
	}
}

func FormatChecks(c model.Checks, pal Palette) string {
	if pal == nil {
		pal = PlainPalette{}
	}
	if c.State == "" && c.Total() == 0 {
		return pal.Gray("—")
	}
	state := strings.ToUpper(c.State)
	label, color := checkLabel(state, pal)
	detail := checkDetail(c)
	if detail == "" {
		return color(label)
	}
	return color(label) + " " + pal.Gray(detail)
}

func checkLabel(state string, pal Palette) (string, func(string) string) {
	switch state {
	case "SUCCESS":
		return "pass", pal.Green
	case "FAILURE", "ERROR":
		return "fail", pal.Red
	case "PENDING", "EXPECTED":
		return "pending", pal.Yellow
	default:
		return strings.ToLower(state), pal.Gray
	}
}

func checkDetail(c model.Checks) string {
	state := strings.ToUpper(c.State)
	if state == "FAILURE" || state == "ERROR" {
		if len(c.FailedNames) > 0 {
			names := c.FailedNames
			if len(names) > 3 {
				names = names[:3]
			}
			return "(" + strings.Join(names, ", ") + ")"
		}
	}
	if c.Total() == 0 {
		return ""
	}
	counts := fmt.Sprintf("%d/%d", c.Passed, c.Counted())
	if state == "FAILURE" || state == "ERROR" {
		counts = fmt.Sprintf("%d/%d", c.Failed, c.Counted())
	}
	if c.Skipped > 0 {
		counts += fmt.Sprintf(", %d skipped", c.Skipped)
	}
	return "(" + counts + ")"
}

func FormatReviews(r model.Reviews, pal Palette) string {
	if pal == nil {
		pal = PlainPalette{}
	}
	decision := strings.ToUpper(r.Decision)
	label, color := reviewLabel(decision, pal)
	people := reviewPeople(r, decision)
	if people == "" {
		if decision == "" {
			return pal.Gray("—")
		}
		return color(label)
	}
	return color(label) + " " + pal.Gray("("+people+")")
}

func reviewLabel(decision string, pal Palette) (string, func(string) string) {
	switch decision {
	case "APPROVED":
		return "approved", pal.Green
	case "CHANGES_REQUESTED":
		return "changes requested", pal.Red
	case "REVIEW_REQUIRED":
		return "review required", pal.Yellow
	case "":
		return "—", pal.Gray
	default:
		return strings.ToLower(strings.ReplaceAll(decision, "_", " ")), pal.Gray
	}
}

func reviewPeople(r model.Reviews, decision string) string {
	switch decision {
	case "APPROVED":
		return joinLimited(r.Approved, 3)
	case "CHANGES_REQUESTED":
		return joinLimited(r.ChangesRequested, 3)
	case "REVIEW_REQUIRED":
		return joinLimited(r.Pending, 3)
	default:
		var all []string
		all = append(all, r.Approved...)
		all = append(all, r.ChangesRequested...)
		all = append(all, r.Pending...)
		return joinLimited(all, 3)
	}
}

func joinLimited(names []string, n int) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) > n {
		return strings.Join(names[:n], ", ") + ", …"
	}
	return strings.Join(names, ", ")
}

func visibleLen(s string) int {
	return len(stripANSI(s))
}

func padVisible(s string, width int) string {
	n := visibleLen(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			if i < len(s) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
