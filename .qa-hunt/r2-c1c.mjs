setTimeout(() => process.exit(9), 28_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 30; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
await login(ev);
await ev(`(()=>{window.__prompt=[];window.prompt=(a,b)=>{window.__prompt.push(String(a).slice(0,20));return null};return true})()`);
await ev("document.querySelector('#btn-join-top').click()");
for (let i = 0; i < 30; i++) { await sleep(300); if (await ev("document.querySelector('#join-cmd').value.indexOf('hr_') === 0")) break; }
await ev("window.__prompt=[]");
// 真实鼠标点 #btn-copy，并额外挂 window.onerror 做「独立第二观测通道」
await ev("window.__e2=[];window.addEventListener('error',e=>window.__e2.push(e.message))");
const box = await ev("(()=>{const r=document.querySelector('#btn-copy').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
await send("Input.dispatchMouseEvent", { type: "mousePressed", x: box.x, y: box.y, button: "left", clickCount: 1 }, sid);
await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: box.x, y: box.y, button: "left", clickCount: 1 }, sid);
await sleep(1200);
console.log("round3(real mouse):", JSON.stringify(await ev(`({ btn: document.querySelector('#btn-copy').textContent, fallbackHidden: document.querySelector('#copy-fallback').classList.contains('hidden'), prompts: window.__prompt.slice(), winErr: window.__e2, toast: document.querySelector('#toast').textContent })`)));
await shot("C1-round3-realclick.png");
// 对照：在同一页面点 #btn-join-top 之外的两处（已做），再补一次 copyAgentId 对照轮 2
await ev("document.querySelector('#dlg-join').close()");
await sleep(400);
await ev("window.__prompt=[]");
const b2 = await ev("(()=>{const r=document.querySelectorAll('.agent-card .aid')[1].getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
await send("Input.dispatchMouseEvent", { type: "mousePressed", x: b2.x, y: b2.y, button: "left", clickCount: 1 }, sid);
await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: b2.x, y: b2.y, button: "left", clickCount: 1 }, sid);
await sleep(900);
console.log("ctrl-copyAgentId-r2:", JSON.stringify(await ev(`({ prompts: window.__prompt.slice(), toast: document.querySelector('#toast').textContent })`)));
process.exit(0);
