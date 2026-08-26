const LIB = [
  "lib/gss.js",
  "lib/parse.js",
  "lib/format.js",
  "lib/readiness.js",
  "lib/queries.js",
  "lib/github.js",
];
importScripts(...LIB);

const cache = new Map();
const CACHE_MS = 20_000;

async function settings() {
  const stored = await chrome.storage.sync.get({ token: "", apiBase: "https://api.github.com" });
  return stored;
}

async function loadStack(owner, repo, number) {
  const key = `${owner}/${repo}#${number}`;
  const hit = cache.get(key);
  if (hit && Date.now() - hit.at < CACHE_MS) return hit.stack;
  const { token, apiBase } = await settings();
  const client = new gss.GitHubClient({ token, apiBase });
  const stack = await client.loadStackByPR(owner, repo, number);
  stack.pullRequests = gss.annotateStack(stack);
  cache.set(key, { at: Date.now(), stack });
  return stack;
}

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (!msg || msg.type !== "gss:loadStack") return;
  loadStack(msg.owner, msg.repo, msg.number)
    .then((stack) => sendResponse({ ok: true, stack }))
    .catch((err) => {
      const needsToken = err && (err.status === 401 || err.status === 403 || /Bad credentials|Requires authentication/i.test(err.message || ""));
      sendResponse({ ok: false, error: err.message || String(err), needsToken: !!needsToken, status: err.status || 0 });
    });
  return true;
});

chrome.runtime.onInstalled.addListener((details) => {
  if (details.reason === "install") chrome.runtime.openOptionsPage();
});
