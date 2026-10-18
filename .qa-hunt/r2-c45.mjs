setTimeout(() => process.exit(9), 28_000).unref();
import { connect, sleep, login } from "/root/code/homer/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot, events } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Network.enable", { maxPostDataSize: 1000 }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 30; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
console.log("loggedIn:", await login(ev));

/* ============ C4: 用 CDP Network 层阻断 /api/auth/logout（不碰 window.fetch） ============ */
const cookie = await ev("document.cookie");
console.log("[C4] cookie:", String(cookie).slice(0, 60));
const base = await ev("(()=>{fetch('/api/console',{credentials:'same-origin'}).then(r=>window.__base=r.status);return 'sent'})()");
await sleep(1200);
console.log("[C4] baseline /api/console status (before logout):", await ev("window.__base"));
await send("Network.setBlockedURLs", { urls: ["*auth/logout*"] }, sid);
events.length = 0;
await ev("document.querySelector('#btn-logout').click()");
await sleep(2500);
const c4 = await ev(`({
  workspaceHidden: document.querySelector('#workspace').classList.contains('hidden'),
  gateLoginHidden: document.querySelector('#gate-login').classList.contains('hidden'),
  errLogin: document.querySelector('#err-login').textContent,
  toast: document.querySelector('#toast').textContent
})`);
console.log("[C4] after blocked logout:", JSON.stringify(c4));
await shot("C4-logout-blocked-no-error.png");
// 解封后从页面用 same-origin 凭据探服务端会话（独立判据：不由前端 setState 决定）
await send("Network.setBlockedURLs", { urls: [] }, sid);
await ev("(()=>{fetch('/api/console',{credentials:'same-origin'}).then(r=>window.__c=r.status);return 1})()");
await sleep(1500);
console.log("[C4] SERVER session /api/console status AFTER logout (same cookie):", await ev("window.__c"));
await shot("C4-server-session-still-200.png");

/* ============ C5: CDP 事件计数真实 /api/agents 请求数 ============ */
console.log("loggedIn2:", await login(ev));
await sleep(800);
// control：单点一次
events.length = 0;
await ev("document.querySelector('#btn-refresh').click()");
await sleep(1500);
const ctrl = events.filter(e => e.method === "Network.requestWillBeSent" && /\/api\/agents/.test(e.params.request.url)).length;
console.log("[C5-control] single click -> /api/agents requests:", ctrl);
// 实验：连点 3 次
events.length = 0;
await ev("(()=>{const b=document.querySelector('#btn-refresh');b.click();b.click();b.click();return 1})()");
await sleep(300);
const fb = await ev(`({ busyHidden: document.querySelector('#busy').classList.contains('hidden'), btnDisabled: document.querySelector('#btn-refresh').disabled, ariaBusy: document.querySelector('#btn-refresh').getAttribute('aria-busy'), refreshedAt: document.querySelector('#refreshed-at').textContent })`);
console.log("[C5] t+0.3s feedback:", JSON.stringify(fb));
await sleep(2000);
const exp = events.filter(e => e.method === "Network.requestWillBeSent" && /\/api\/agents/.test(e.params.request.url)).length;
console.log("[C5-experiment] triple click -> /api/agents requests:", exp);
await shot("C5-triple-click-no-feedback.png");
process.exit(0);
