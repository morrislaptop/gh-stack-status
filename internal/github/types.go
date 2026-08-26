package github

type gqlStackByPRData struct {
	Repository *struct {
		PullRequest *gqlPRWithStack `json:"pullRequest"`
	} `json:"repository"`
}

type gqlPRWithStack struct {
	gqlPR
	Stack *gqlStack `json:"stack"`
}

type gqlStack struct {
	Number      int    `json:"number"`
	Size        int    `json:"size"`
	BaseRefName string `json:"baseRefName"`
	Entries     struct {
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
		Nodes []gqlStackEntry `json:"nodes"`
	} `json:"entries"`
}

type gqlStackEntry struct {
	Position    int    `json:"position"`
	PullRequest *gqlPR `json:"pullRequest"`
}

type gqlPR struct {
	Number           int     `json:"number"`
	Title            string  `json:"title"`
	URL              string  `json:"url"`
	State            string  `json:"state"`
	IsDraft          bool    `json:"isDraft"`
	HeadRefName      string  `json:"headRefName"`
	ReviewDecision   *string `json:"reviewDecision"`
	LatestReviews    struct {
		Nodes []struct {
			Author *struct {
				Login string `json:"login"`
			} `json:"author"`
			State string `json:"state"`
		} `json:"nodes"`
	} `json:"latestReviews"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer *gqlReviewer `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	StatusCheckRollup *gqlRollup `json:"statusCheckRollup"`
}

type gqlReviewer struct {
	Typename     string `json:"__typename"`
	Login        string `json:"login"`
	Name         string `json:"name"`
	CombinedSlug string `json:"combinedSlug"`
}

func (r *gqlReviewer) DisplayName() string {
	if r == nil {
		return ""
	}
	if r.Login != "" {
		return r.Login
	}
	if r.CombinedSlug != "" {
		return r.CombinedSlug
	}
	return r.Name
}

type gqlRollup struct {
	State    string `json:"state"`
	Contexts struct {
		Nodes []gqlContext `json:"nodes"`
	} `json:"contexts"`
}

type gqlContext struct {
	Typename   string  `json:"__typename"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Conclusion *string `json:"conclusion"`
	Context    string  `json:"context"`
	State      string  `json:"state"`
}

type restStack struct {
	Number int `json:"number"`
	Base   struct {
		Ref string `json:"ref"`
	} `json:"base"`
	PullRequests []restPR `json:"pull_requests"`
}

type restPR struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Draft  bool   `json:"draft"`
	Head   struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
}
