package github

const checkContextFields = `
      __typename
      ... on CheckRun {
        name
        status
        conclusion
        startedAt
        completedAt
        checkSuite {
          workflowRun {
            workflow { name }
          }
        }
      }
      ... on StatusContext {
        context
        state
        createdAt
      }
`

const prStatusFragment = `
fragment PRStatus on PullRequest {
  number
  title
  url
  state
  isDraft
  headRefName
  baseRefName
  isCrossRepository
  headRepositoryOwner { login }
  mergeable
  mergeStateStatus
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
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
` + checkContextFields + `
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

// comparisonsPrefix starts the batched base-vs-head comparison query. One
// aliased ref/compare pair is appended per pull request.
const comparisonsPrefix = `query Comparisons($owner: String!, $name: String!`

// prContextsQuery fetches additional pages of a single pull request's check contexts.
const prContextsQuery = `
query PRContexts($owner: String!, $name: String!, $number: Int!, $cursor: String!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      statusCheckRollup {
        contexts(first: 100, after: $cursor) {
          pageInfo {
            hasNextPage
            endCursor
          }
          nodes {
` + checkContextFields + `
          }
        }
      }
    }
  }
}
`

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
