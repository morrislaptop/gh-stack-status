package github

const prStatusFragment = `
fragment PRStatus on PullRequest {
  number
  title
  url
  state
  isDraft
  headRefName
  reviewDecision
  latestReviews(first: 50) {
    nodes {
      author { login }
      state
    }
  }
  reviewRequests(first: 50) {
    nodes {
      requestedReviewer {
        __typename
        ... on User { login }
        ... on Team { name combinedSlug }
      }
    }
  }
  statusCheckRollup {
    state
    contexts(first: 100) {
      nodes {
        __typename
        ... on CheckRun {
          name
          status
          conclusion
        }
        ... on StatusContext {
          context
          state
        }
      }
    }
  }
}
`

const stackByPRQuery = prStatusFragment + `
query StackByPR($owner: String!, $name: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      ...PRStatus
      stack {
        number
        size
        baseRefName
        entries(first: 50, after: $cursor) {
          pageInfo {
            hasNextPage
            endCursor
          }
          nodes {
            position
            pullRequest {
              ...PRStatus
            }
          }
        }
      }
    }
  }
}
`

const prsByNumbersPrefix = prStatusFragment + `
query PRsByNumbers($owner: String!, $name: String!`

const branchPRQuery = `
query BranchPR($owner: String!, $name: String!, $qualifiedName: String!) {
  repository(owner: $owner, name: $name) {
    ref(qualifiedName: $qualifiedName) {
      associatedPullRequests(first: 10, orderBy: {field: UPDATED_AT, direction: DESC}) {
        nodes {
          number
          state
          headRefName
        }
      }
    }
  }
}
`
