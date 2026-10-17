setTimeout(() => process.exit(9), 28_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot, events } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Network.enable", { maxPostDataSize: 1000 }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 25; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
console.log("loggedIn:", await login(ev));
await sleep(1000);
const count = () => events.filter(e => e.method === "Network.requestWillBeSent" && /\/api\/agents(\?|$)/.test(e.params.request.url)).length;
// control-1: 单点
events.length = 0;
await ev("document.querySelector('#btn-refresh').click()");
await sleep(1200);
console.log("[control-1] 1 click ->", count(), "/api/agents");
// 实验-1: 连点 3 次 + t+0.3s 反馈快照
events.length = 0;
await ev("(()=>{const b=document.querySelector('#btn-refresh');b.click();b.click();b.click();return 1})()");
await sleep(300);
console.log("[exp-1] t+0.3s:", JSON.stringify(await ev(`({ busyHidden: document.querySelector('#busy').classList.contains('hidden'), btnDisabled: document.querySelector('#btn-refresh').disabled, ariaBusy: document.querySelector('#btn-refresh').getAttribute('aria-busy'), refreshedAt: document.querySelector('#refreshed-at').textContent })`)));
await sleep(1800);
console.log("[exp-1] 3 clicks ->", count(), "/api/agents");
await shot("C5-a-triple-no-feedback.png");
// control-2: 再单点
events.length = 0;
await ev("document.querySelector('#btn-refresh').click()");
await sleep(1200);
console.log("[control-2] 1 click ->", count(), "/api/agents");
// 实验-2: 真实鼠标连点 3 次（排除 .click() 伪影）
events.length = 0;
const box = await ev("(()=>{const r=document.querySelector('#btn-refresh').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
for (let i = 0; i < 3; i++) {
  await send("Input.dispatchMouseEvent", { type: "mousePressed", x: box.x, y: box.y, button: "left", clickCount: 1 }, sid);
  await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: box.x, y: box.y, button: "left", clickCount: 1 }, sid);
}
await sleep(1800);
console.log("[exp-2] real mouse 3 clicks ->", count(), "/api/agents");
await shot("C5-b-triple-mouse.png");
process.exit(0);
