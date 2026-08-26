package resolve

import (
	"fmt"
	"testing"

	"github.com/morrislaptop/gh-stack-status/internal/model"
)

func TestParsePRURL(t *testing.T) {
	owner, repo, n, ok := ParsePRURL("https://github.com/acme/app/pull/42/files")
	if !ok || owner != "acme" || repo != "app" || n != 42 {
		t.Fatalf("%s %s %d %v", owner, repo, n, ok)
	}
	if _, _, _, ok := ParsePRURL("not-a-url"); ok {
		t.Fatal("expected false")
	}
}

func TestParseTarget(t *testing.T) {
	cases := []struct {
		in   string
		kind Kind
		n    int
	}{
		{"", KindNone, 0},
		{"12", KindNumber, 12},
		{"https://github.com/o/r/pull/7", KindPRURL, 7},
		{"feature-auth", KindBranch, 0},
	}
	for _, tc := range cases {
		got, err := ParseTarget(tc.in)
		if err != nil {
			t.Fatal(err)
		}
		if got.Kind != tc.kind || got.Number != tc.n {
			t.Fatalf("%q: %+v", tc.in, got)
		}
	}
}

type fakeGH struct {
	byPR     map[int]*model.Stack
	byStack  map[int]*model.Stack
	branches map[string]int
	prErr    map[int]error
	stackErr map[int]error
}

func (f *fakeGH) LoadStackByPR(owner, repo string, number int) (*model.Stack, error) {
	if err, ok := f.prErr[number]; ok {
		return nil, err
	}
	s, ok := f.byPR[number]
	if !ok {
		return nil, fmt.Errorf("pr %d not in stack", number)
	}
	cp := *s
	return &cp, nil
}

func (f *fakeGH) LoadStackByNumber(owner, repo string, stackNumber int) (*model.Stack, error) {
	if err, ok := f.stackErr[stackNumber]; ok {
		return nil, err
	}
	s, ok := f.byStack[stackNumber]
	if !ok {
		return nil, fmt.Errorf("stack #%d not found", stackNumber)
	}
	cp := *s
	return &cp, nil
}

func (f *fakeGH) PRNumberForBranch(owner, repo, branch string) (int, error) {
	n, ok := f.branches[branch]
	if !ok {
		return 0, fmt.Errorf("no pull request found for branch %q", branch)
	}
	return n, nil
}

func sampleStack() *model.Stack {
	return &model.Stack{
		Number: 6,
		Base:   "main",
		PullRequests: []model.PullRequest{
			{Number: 101, Branch: "auth-layer", Position: 1},
			{Number: 102, Branch: "api-endpoints", Position: 2},
		},
	}
}

func TestLoadCurrentBranch(t *testing.T) {
	st := sampleStack()
	gh := &fakeGH{
		byPR:     map[int]*model.Stack{102: st},
		branches: map[string]int{"api-endpoints": 102},
	}
	got, err := Load(gh, Repo{Owner: "o", Name: "r"}, "api-endpoints", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentBranch != "api-endpoints" || got.Number != 6 {
		t.Fatalf("%+v", got)
	}
}

func TestLoadNumberPrefersStack(t *testing.T) {
	st := sampleStack()
	gh := &fakeGH{
		byStack: map[int]*model.Stack{6: st},
		byPR:    map[int]*model.Stack{6: {Number: 99, Base: "other"}},
	}
	got, err := Load(gh, Repo{Owner: "o", Name: "r"}, "main", "6")
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 6 || got.Base != "main" {
		t.Fatalf("%+v", got)
	}
}

func TestLoadNumberFallsBackToPR(t *testing.T) {
	st := sampleStack()
	gh := &fakeGH{
		stackErr: map[int]error{101: fmt.Errorf("stack #101 not found")},
		byPR:     map[int]*model.Stack{101: st},
	}
	got, err := Load(gh, Repo{Owner: "o", Name: "r"}, "auth-layer", "101")
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 6 {
		t.Fatalf("%+v", got)
	}
}

func TestLoadPRURL(t *testing.T) {
	st := sampleStack()
	gh := &fakeGH{byPR: map[int]*model.Stack{102: st}}
	got, err := Load(gh, Repo{Owner: "ignored", Name: "ignored"}, "x", "https://github.com/acme/app/pull/102")
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 6 {
		t.Fatalf("%+v", got)
	}
}

func TestLoadBranch(t *testing.T) {
	st := sampleStack()
	gh := &fakeGH{
		branches: map[string]int{"auth-layer": 101},
		byPR:     map[int]*model.Stack{101: st},
	}
	got, err := Load(gh, Repo{Owner: "o", Name: "r"}, "other", "auth-layer")
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentBranch != "other" {
		t.Fatalf("current branch should remain checkout, got %q", got.CurrentBranch)
	}
}
