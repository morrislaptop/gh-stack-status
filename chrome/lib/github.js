var gss = globalThis.gss || (globalThis.gss = {});

var API_VERSION = "2026-03-10";

function graphqlError(payload) {
  if (!payload || !payload.errors || !payload.errors.length) return null;
  return payload.errors.map(function (e) { return e.message; }).join("; ");
}

function GitHubClient(opts) {
  this.token = (opts && opts.token) || "";
  this.apiBase = (opts && opts.apiBase) || "https://api.github.com";
}

GitHubClient.prototype.headers = function (extra) {
  var h = Object.assign({
    Accept: "application/vnd.github+json",
    "X-GitHub-Api-Version": API_VERSION,
    "Content-Type": "application/json",
  }, extra || {});
  if (this.token) h.Authorization = "Bearer " + this.token;
  return h;
};

GitHubClient.prototype.graphql = async function (query, variables) {
  var res = await fetch(this.apiBase.replace(/\/$/, "") + "/graphql", {
    method: "POST",
    headers: this.headers(),
    body: JSON.stringify({ query: query, variables: variables || {} }),
  });
  var body = await res.json().catch(function () { return {}; });
  if (!res.ok) {
    var msg = body.message || graphqlError(body) || ("HTTP " + res.status);
    var err = new Error(msg);
    err.status = res.status;
    throw err;
  }
  var gqlErr = graphqlError(body);
  if (gqlErr) {
    var e = new Error(gqlErr);
    e.graphql = body.errors;
    throw e;
  }
  return body.data;
};

GitHubClient.prototype.rest = async function (path) {
  var res = await fetch(this.apiBase.replace(/\/$/, "") + "/" + path.replace(/^\//, ""), {
    headers: this.headers(),
  });
  if (res.status === 404) return null;
  var body = await res.json().catch(function () { return {}; });
  if (!res.ok) {
    var err = new Error(body.message || ("HTTP " + res.status));
    err.status = res.status;
    throw err;
  }
  return body;
};

GitHubClient.prototype.fetchRemainingContexts = async function (owner, repo, pr) {
  if (!pr || !pr.statusCheckRollup) return;
  var page = pr.statusCheckRollup.contexts && pr.statusCheckRollup.contexts.pageInfo;
  while (page && page.hasNextPage && page.endCursor) {
    var data = await this.graphql(gss.prContextsQuery, {
      owner: owner,
      name: repo,
      number: pr.number,
      cursor: page.endCursor,
    });
    var next = data && data.repository && data.repository.pullRequest &&
      data.repository.pullRequest.statusCheckRollup &&
      data.repository.pullRequest.statusCheckRollup.contexts;
    if (!next) return;
    pr.statusCheckRollup.contexts.nodes = (pr.statusCheckRollup.contexts.nodes || []).concat(next.nodes || []);
    if (next.pageInfo && next.pageInfo.endCursor === page.endCursor) return;
    page = next.pageInfo;
  }
};

GitHubClient.prototype.queryStackByPR = async function (owner, repo, number) {
  var entries = [];
  var meta = null;
  var first = null;
  var cursor = null;
  for (;;) {
    var data = await this.graphql(gss.stackByPRQuery, {
      owner: owner,
      name: repo,
      number: number,
      cursor: cursor,
    });
    var pr = data && data.repository && data.repository.pullRequest;
    if (!pr) return { stack: null, pr: first };
    if (!first) first = pr;
    if (!pr.stack) break;
    if (!meta) meta = pr.stack;
    entries = entries.concat(pr.stack.entries.nodes || []);
    var page = pr.stack.entries.pageInfo;
    if (!page || !page.hasNextPage || !page.endCursor) break;
    cursor = page.endCursor;
  }
  if (!meta) return { stack: null, pr: first };
  for (var i = 0; i < entries.length; i++) {
    if (entries[i].pullRequest) {
      await this.fetchRemainingContexts(owner, repo, entries[i].pullRequest);
    }
  }
  return { stack: gss.parseGQLStack(meta, entries), pr: first };
};

GitHubClient.prototype.loadStackByPR = async function (owner, repo, number) {
  var gqlErr = null;
  try {
    var got = await this.queryStackByPR(owner, repo, number);
    if (got.stack && got.stack.pullRequests.length) return got.stack;
  } catch (err) {
    gqlErr = err;
  }
  var restStack = await this.rest(
    "repos/" + encodeURIComponent(owner) + "/" + encodeURIComponent(repo) + "/stacks?pull_request=" + number
  );
  var first = Array.isArray(restStack) ? restStack[0] : restStack;
  if (first && first.pull_requests && first.pull_requests.length) {
    return this.hydrateRESTStack(owner, repo, first);
  }
  if (gqlErr) throw gqlErr;
  throw new Error("pull request #" + number + " is not part of a GitHub stack");
};

GitHubClient.prototype.queryPRsByNumbers = async function (owner, repo, numbers) {
  if (!numbers.length) return [];
  var vars = { owner: owner, name: repo };
  var args = ["$owner: String!", "$name: String!"];
  var fields = [];
  numbers.forEach(function (n, i) {
    var alias = "n" + i;
    args.push("$" + alias + ": Int!");
    vars[alias] = n;
    fields.push("    " + alias + ": pullRequest(number: $" + alias + ") { ...PRStatus }");
  });
  var query = gss.prStatusFragment +
    "query PRsByNumbers(" + args.join(", ") + ") {\n  repository(owner: $owner, name: $name) {\n" +
    fields.join("\n") + "\n  }\n}\n";
  var data = await this.graphql(query, vars);
  var repoData = data && data.repository ? data.repository : {};
  var out = [];
  for (var i = 0; i < numbers.length; i++) {
    var pr = repoData["n" + i];
    if (!pr) continue;
    await this.fetchRemainingContexts(owner, repo, pr);
    out.push(gss.prToModel(pr, i + 1));
  }
  return out;
};

GitHubClient.prototype.hydrateRESTStack = async function (owner, repo, rest) {
  var numbers = (rest.pull_requests || []).map(function (p) { return p.number; });
  var prs = await this.queryPRsByNumbers(owner, repo, numbers);
  var byNum = {};
  prs.forEach(function (pr) { byNum[pr.number] = pr; });
  var stack = {
    number: rest.number,
    base: rest.base && rest.base.ref ? rest.base.ref : "",
    pullRequests: [],
  };
  (rest.pull_requests || []).forEach(function (restPR, i) {
    var pr = byNum[restPR.number] || {
      number: restPR.number,
      branch: restPR.head && restPR.head.ref ? restPR.head.ref : "",
      state: String(restPR.state || "").toUpperCase(),
      draft: !!restPR.draft,
      title: "",
      url: "",
      checks: { state: "", passed: 0, failed: 0, pending: 0, skipped: 0, failedNames: [] },
      reviews: { decision: "", approved: [], changesRequested: [], pending: [] },
      rebase: { status: "UNKNOWN", mergeable: "", mergeStateStatus: "" },
    };
    pr.position = i + 1;
    stack.pullRequests.push(pr);
  });
  return stack;
};

gss.GitHubClient = GitHubClient;

if (typeof module !== "undefined" && module.exports) module.exports = gss;
