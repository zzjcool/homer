setTimeout(() => process.exit(9), 30_000).unref();
import { connect, sleep } from "./repro2-lib.mjs";
const B = process.argv[2] || "http://hub:7760";
const { send, ev, sid } = await connect();
await send("Page.enable", {}, sid);
await send("Page.navigate", { url: B + "/" }, sid);
await sleep(3000);
console.log("url:", await ev("location.href"));
// 关键：安全上下文判定
console.log("isSecureContext:", await ev("window.isSecureContext"), "clipboard typeof:", await ev("typeof navigator.clipboard"));
const st = await ev("(() => {const w=document.querySelector(#workspace),g=document.querySelector(#gate-login);return {workspaceHidden: w?w.classList.contains(hidden):null, gateHidden: g?g.classList.contains(hidden):null}})()");
console.log("pre-login:", JSON.stringify(st));
if (st.workspaceHidden !== false) {
  await ev("document.querySelector(#in-login-pw).value=qa-admin-password");
  await ev("document.querySelector(#btn-login).click()");
  for (let i=0;i<40;i++){ await sleep(300); if (await ev("!document.querySelector(#workspace).classList.contains(hidden)")) break; }
}
console.log("logged-in:", await ev("!document.querySelector(#workspace).classList.contains(hidden)"));
console.log("cards:", await ev("document.querySelectorAll(.agent-card).length"));
