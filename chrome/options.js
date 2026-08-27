const tokenEl = document.getElementById("token");
const apiBaseEl = document.getElementById("apiBase");
const statusEl = document.getElementById("status");

chrome.storage.sync.get({ token: "", apiBase: "https://api.github.com" }).then((s) => {
  tokenEl.value = s.token;
  apiBaseEl.value = s.apiBase || "https://api.github.com";
});

document.getElementById("save").addEventListener("click", async () => {
  await chrome.storage.sync.set({
    token: tokenEl.value.trim(),
    apiBase: (apiBaseEl.value.trim() || "https://api.github.com").replace(/\/$/, ""),
  });
  statusEl.textContent = "Saved. Reload the GitHub tab to refresh the stack map.";
});
