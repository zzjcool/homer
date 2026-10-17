setTimeout(() => process.exit(9), 30_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot } = await connect();
await send("Runtime.enable", {}, sid);
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 40; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
console.log("[pre] loggedIn:", await login(ev));

// ---- 我的判据 1（不复用 window.__err）：把 window.prompt 打桩计数，观察三处复制是否都走了「降级 prompt」。
// copyCommand / copyAgentId 在 !navigator.clipboard 时必定调 window.prompt（源码 2163/2190）。
// copyJoin 若也走降级，必须同样弹 prompt + 显示 #copy-fallback。
await ev(`(()=>{window.__prompt=[];const o=window.prompt;window.prompt=(a,b)=>{window.__prompt.push(String(a).slice(0,20));return null};return true})()`);

// ---- 判据 2（独立观测剪贴板真值）：execCommand('copy') 返回值 + 原 navigator.clipboard 探针
console.log("[pre] clipboard typeof:", await ev("typeof navigator.clipboard"), "| isSecureContext:", await ev("window.isSecureContext"));

// ============ 实验 A：copyJoin（接入命令对话框） ============
await ev("document.querySelector('#btn-join-top').click()");
for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#join-cmd').value.indexOf('hr_') === 0")) break; }
const cmd = await ev("document.querySelector('#join-cmd').value");
console.log("[A] join cmd:", String(cmd).slice(0, 60), "...");
// 用真实鼠标点击（Input.dispatchMouseEvent），不走 .click() 捷径
const box = await ev("(()=>{const r=document.querySelector('#btn-copy').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
await send("Input.dispatchMouseEvent", { type: "mousePressed", x: box.x, y: box.y, button: "left", clickCount: 1 }, sid);
await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: box.x, y: box.y, button: "left", clickCount: 1 }, sid);
await sleep(1200);
const a = await ev(`({
  btn: document.querySelector('#btn-copy').textContent,
  fallbackHidden: document.querySelector('#copy-fallback').classList.contains('hidden'),
  prompts: window.__prompt.slice(),
  selection: String(document.getSelection()).slice(0,12),
  execCopy: (()=>{try{return document.execCommand('copy')}catch(e){return 'EXC:'+e.message}})()
})`);
console.log("[A] copyJoin  ->", JSON.stringify(a));
await shot("C1-A-copyjoin-after-click.png");
await ev("window.__prompt=[]");
await ev("document.querySelector('#dlg-join').close()");
await sleep(500);

// ============ 实验 B：copyAgentId（有守卫）对照 ============
const aid = await ev("document.querySelector('.agent-card .aid').dataset.agentId");
const b2 = await ev("(()=>{const r=document.querySelector('.agent-card .aid').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
await send("Input.dispatchMouseEvent", { type: "mousePressed", x: b2.x, y: b2.y, button: "left", clickCount: 1 }, sid);
await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: b2.x, y: b2.y, button: "left", clickCount: 1 }, sid);
await sleep(1000);
const b = await ev(`({ aid: document.querySelector('.agent-card .aid').textContent, prompts: window.__prompt.slice(), toast: document.querySelector('#toast').textContent })`);
console.log("[B] copyAgentId ->", JSON.stringify(b), "(expected aid=已复制 or prompts 非空)");
await shot("C1-B-copyagentid.png");
await ev("window.__prompt=[]");

// ============ 实验 C：copyCommand（有守卫）对照 ============
const c = await ev(`(()=>{ try { copyCommand('__R2_PROBE__'); return 'no-throw'; } catch(e){ return 'THROW:'+e.message; } })()`);
await sleep(800);
const c2 = await ev(`({ prompts: window.__prompt.slice(), toast: document.querySelector('#toast').textContent })`);
console.log("[C] copyCommand ->", c, JSON.stringify(c2), "(expected prompts 非空 = 走降级，不抛)");
await shot("C1-C-copycommand.png");

// ============ 实验 D：repeat A（对照轮 2）============
await ev("document.querySelector('#btn-join-top').click()");
for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#join-cmd').value.indexOf('hr_') === 0")) break; }
await ev("window.__prompt=[]");
const d1 = await ev(`(()=>{ try { copyJoin('join-cmd','btn-copy'); return 'no-throw'; } catch(e){ return 'THROW:'+e.name+': '+e.message; } })()`);
await sleep(1000);
const d2 = await ev(`({ btn: document.querySelector('#btn-copy').textContent, fallbackHidden: document.querySelector('#copy-fallback').classList.contains('hidden'), prompts: window.__prompt.slice() })`);
console.log("[D] copyJoin round2 ->", d1, JSON.stringify(d2));
process.exit(0);
