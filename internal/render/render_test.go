package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/morrislaptop/gh-stack-status/internal/model"
)

func sample() *model.Stack {
	return &model.Stack{
		Number:        6,
		Base:          "main",
		CurrentBranch: "api-endpoints",
		PullRequests: []model.PullRequest{
			{
				Number: 101, Title: "auth", URL: "https://example.com/101",
				Branch: "auth-layer", State: "OPEN", Position: 1,
				Checks:  model.Checks{State: "SUCCESS", Passed: 12, Failed: 0, Pending: 0},
				Reviews: model.Reviews{Decision: "APPROVED", Approved: []string{"alice"}},
				Rebase:  model.Rebase{Status: "UP_TO_DATE", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
			},
			{
				Number: 102, Title: "api", URL: "https://example.com/102",
				Branch: "api-endpoints", State: "OPEN", Position: 2,
				Checks:  model.Checks{State: "FAILURE", Passed: 7, Failed: 1, Pending: 0, FailedNames: []string{"lint"}},
				Reviews: model.Reviews{Decision: "CHANGES_REQUESTED", ChangesRequested: []string{"bob"}},
				Rebase: model.Rebase{
					Status: "BEHIND", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN",
					Comparison: &model.Comparison{Status: "BEHIND", AheadBy: 0, BehindBy: 12},
				},
			},
			{
				Number: 103, Title: "ui", URL: "https://example.com/103",
				Branch: "frontend", State: "OPEN", Position: 3,
				Checks:  model.Checks{State: "PENDING", Passed: 3, Failed: 0, Pending: 5},
				Reviews: model.Reviews{Decision: "REVIEW_REQUIRED", Pending: []string{"carol"}},
				Rebase:  model.Rebase{Status: "CONFLICT", Mergeable: "CONFLICTING", MergeStateStatus: "DIRTY"},
			},
		},
	}
}

func TestWriteTable(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, sample(), PlainPalette{}, Options{}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "Stack #6  targeting main  3 PRs") {
		t.Fatalf("header: %s", got)
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	// top of stack listed first
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "frontend") || !strings.Contains(joined, "#103") {
		t.Fatalf("missing top PR:\n%s", got)
	}
	frontIdx := strings.Index(got, "frontend")
	authIdx := strings.Index(got, "auth-layer")
	if frontIdx < 0 || authIdx < 0 || frontIdx > authIdx {
		t.Fatalf("expected frontend above auth-layer:\n%s", got)
	}
	if !strings.Contains(got, "»") {
		t.Fatalf("missing current marker:\n%s", got)
	}
	if !strings.Contains(got, "pass (12/12)") {
		t.Fatalf("pass counts:\n%s", got)
	}
	if !strings.Contains(got, "fail (lint)") {
		t.Fatalf("fail names:\n%s", got)
	}
	if !strings.Contains(got, "pending (3/8)") {
		t.Fatalf("pending counts:\n%s", got)
	}
	if !strings.Contains(got, "behind") {
		t.Fatalf("rebase behind:\n%s", got)
	}
	if !strings.Contains(got, "conflict") {
		t.Fatalf("rebase conflict:\n%s", got)
	}
	if !strings.Contains(got, "up to date") {
		t.Fatalf("rebase up to date:\n%s", got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "main") {
		t.Fatalf("trunk at bottom:\n%s", got)
	}
}

func TestWriteShort(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, sample(), PlainPalette{}, Options{Short: true}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	want := []string{
		"#103  conflict  pending (3/8)  review required (carol)",
		"#102  behind  fail (lint)  changes requested (bob)",
		"#101  up to date  pass (12/12)  approved (alice)",
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines: %q", got)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Fatalf("line %d:\n got %q\nwant %q", i, lines[i], w)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, sample(), PlainPalette{}, Options{JSON: true}); err != nil {
		t.Fatal(err)
	}
	var out jsonStack
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Number != 6 || out.Base != "main" || out.CurrentBranch != "api-endpoints" {
		t.Fatalf("%+v", out)
	}
	if len(out.PullRequests) != 3 || out.PullRequests[0].Number != 101 {
		t.Fatalf("json should be bottom-to-top, got %#v", out.PullRequests)
	}
	if !out.PullRequests[1].IsCurrent {
		t.Fatal("expected current on #102")
	}
	if out.PullRequests[1].Checks.FailedNames[0] != "lint" {
		t.Fatalf("failed names %v", out.PullRequests[1].Checks.FailedNames)
	}
	if out.PullRequests[1].Rebase.Status != "BEHIND" {
		t.Fatalf("rebase %+v", out.PullRequests[1].Rebase)
	}
	cmp := out.PullRequests[1].Rebase.Comparison
	if cmp == nil || cmp.BehindBy != 12 || cmp.Status != "BEHIND" {
		t.Fatalf("comparison %+v", cmp)
	}
	if out.PullRequests[0].Rebase.Comparison != nil {
		t.Fatalf("comparison should be null when GitHub could not compare: %+v", out.PullRequests[0].Rebase.Comparison)
	}
}

func TestFormatChecksNone(t *testing.T) {
	if got := FormatChecks(model.Checks{}, PlainPalette{}); got != "—" {
		t.Fatalf("%q", got)
	}
}

func TestWriteTableCurrentMarkerOnMiddlePR(t *testing.T) {
	var buf bytes.Buffer
	_ = Write(&buf, sample(), PlainPalette{}, Options{})
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "api-endpoints") && !strings.Contains(line, "»") {
			t.Fatalf("current PR should have marker: %q", line)
		}
		if strings.Contains(line, "frontend") && strings.Contains(line, "»") {
			t.Fatalf("non-current should not have marker: %q", line)
		}
	}
}
