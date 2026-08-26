package resolve

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/morrislaptop/gh-stack-status/internal/model"
)

type GitHub interface {
	LoadStackByPR(owner, repo string, number int) (*model.Stack, error)
	LoadStackByNumber(owner, repo string, stackNumber int) (*model.Stack, error)
	PRNumberForBranch(owner, repo, branch string) (int, error)
}

type Repo struct {
	Owner string
	Name  string
}

type Target struct {
	Kind        Kind
	Number      int
	Branch      string
	Owner       string
	Repo        string
	Raw         string
}

type Kind int

const (
	KindNone Kind = iota
	KindNumber
	KindPRURL
	KindBranch
)

func ParseTarget(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{Kind: KindNone}, nil
	}
	if owner, repo, n, ok := ParsePRURL(raw); ok {
		return Target{Kind: KindPRURL, Number: n, Owner: owner, Repo: repo, Raw: raw}, nil
	}
	if n, ok := ParsePositiveInt(raw); ok {
		return Target{Kind: KindNumber, Number: n, Raw: raw}, nil
	}
	return Target{Kind: KindBranch, Branch: raw, Raw: raw}, nil
}

func ParsePositiveInt(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func ParsePRURL(raw string) (owner, repo string, number int, ok bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", 0, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// owner/repo/pull/123
	if len(parts) < 4 || parts[2] != "pull" {
		return "", "", 0, false
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil || n <= 0 {
		return "", "", 0, false
	}
	return parts[0], parts[1], n, true
}

func Load(gh GitHub, repo Repo, currentBranch, targetArg string) (*model.Stack, error) {
	t, err := ParseTarget(targetArg)
	if err != nil {
		return nil, err
	}

	owner, name := repo.Owner, repo.Name
	var stack *model.Stack

	switch t.Kind {
	case KindNone:
		if currentBranch == "" {
			return nil, fmt.Errorf("not on a named branch; pass a stack number, pull request, or branch")
		}
		n, err := gh.PRNumberForBranch(owner, name, currentBranch)
		if err != nil {
			return nil, err
		}
		stack, err = gh.LoadStackByPR(owner, name, n)
		if err != nil {
			return nil, err
		}
	case KindPRURL:
		owner, name = t.Owner, t.Repo
		stack, err = gh.LoadStackByPR(owner, name, t.Number)
		if err != nil {
			return nil, err
		}
	case KindNumber:
		stack, err = gh.LoadStackByNumber(owner, name, t.Number)
		if err != nil {
			stack, err = gh.LoadStackByPR(owner, name, t.Number)
			if err != nil {
				return nil, fmt.Errorf("no stack or pull request #%d in %s/%s", t.Number, owner, name)
			}
		}
	case KindBranch:
		n, err := gh.PRNumberForBranch(owner, name, t.Branch)
		if err != nil {
			return nil, err
		}
		stack, err = gh.LoadStackByPR(owner, name, n)
		if err != nil {
			return nil, err
		}
	}

	if stack == nil {
		return nil, fmt.Errorf("stack not found")
	}
	stack.CurrentBranch = currentBranch
	return stack, nil
}
