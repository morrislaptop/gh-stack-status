/* shared namespace for classic content scripts + Node tests */
var gss = globalThis.gss || (globalThis.gss = {});
if (typeof module !== "undefined" && module.exports) {
  module.exports = gss;
}
