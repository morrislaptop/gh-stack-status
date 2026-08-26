var gss = globalThis.gss || (globalThis.gss = {});

var checkContextFields = `
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
`;

gss.prStatusFragment = `
fragment PRStatus on PullRequest {
  number
  title
  url
  state
  isDraft
  headRefName
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
`;

gss.stackByPRQuery = gss.prStatusFragment + `
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
`;

gss.prContextsQuery = `
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
`;

if (typeof module !== "undefined" && module.exports) module.exports = gss;
