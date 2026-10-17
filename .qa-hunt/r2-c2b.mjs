setTimeout(() => process.exit(9), 55_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
console.log("loggedIn:", await login(ev));
await sleep(600);
const ID = process.argv[2] || "b6675d68ddf7-7920";
// 用「处理冲突」入口（该机器 conf=1），走 resolve 对话框 → 它有「记录并立即执行」= dispatch
const r = await ev(`(()=>{
  const card=[...document.querySelectorAll('.agent-card')].find(c=>c.getAttribute('data-agent-id')===${JSON.stringify(ID)});
  if(!card) return 'NOCARD';
  const b=[...card.querySelectorAll('button')].find(x=>x.textContent.trim()==='处理冲突');
  if(!b) return 'NOBTN:'+[...card.querySelectorAll('button')].map(x=>x.textContent.trim()).join('/');
  b.click(); return 'clicked';})()`);
console.log("[step] resolve click:", r);
for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#dlg-confirm').open && document.querySelector('#confirm-choices').children.length>0")) break; }
console.log("[pre] open:", await ev("document.querySelector('#dlg-confirm').open"), "| kind:", await ev("app.pendingAction&&app.pendingAction.kind"), "| choices:", await ev("document.querySelector('#confirm-choices').children.length"));
console.log("[pre] ok:", await ev("document.querySelector('#btn-confirm-ok').textContent"), "| alt:", await ev("document.querySelector('#btn-confirm-alt').textContent"), "| altHidden:", await ev("document.querySelector('#btn-confirm-alt').classList.contains('hidden')"));
await shot("C2b-a-resolve-open.png");
// 选一个冲突的 resolution radio
await ev(`(()=>{const r=document.querySelector('#confirm-choices input[type=radio]');if(!r)return 'NORADIO';r.checked=true;r.dispatchEvent(new Event('change',{bubbles:true}));return 'radio-checked'})()`);
await sleep(600);
console.log("[pre] after radio -> ok:", await ev("document.querySelector('#btn-confirm-ok').textContent"), "| alt:", await ev("document.querySelector('#btn-confirm-alt').textContent"));
// 点「记录并立即执行」（ALT）→ 走 executeDispatchOne(stay=true) → beginDispatchWait
await ev("document.querySelector('#btn-confirm-alt').click()");
await sleep(2500);
console.log("[wait] ->", JSON.stringify(await ev(`({
  open: document.querySelector('#dlg-confirm').open,
  okText: document.querySelector('#btn-confirm-ok').textContent,
  okDisabled: document.querySelector('#btn-confirm-ok').disabled,
  cancelDisabled: document.querySelector('#btn-confirm-cancel').disabled,
  backHidden: document.querySelector('#btn-confirm-back').classList.contains('hidden'),
  step: app.scopeStep, kind: app.pendingAction && app.pendingAction.kind,
  desc: document.querySelector('#confirm-desc').textContent,
  title: document.querySelector('#confirm-title').textContent
})`)));
await shot("C2b-b-waiting-REAL-slow.png");
// 真实鼠标点「取消」
const bx = await ev("(()=>{const r=document.querySelector('#btn-confirm-cancel').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
await send("Input.dispatchMouseEvent", { type: "mousePressed", x: bx.x, y: bx.y, button: "left", clickCount: 1 }, sid);
await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: bx.x, y: bx.y, button: "left", clickCount: 1 }, sid);
await sleep(1500);
console.log("[after cancel click] ->", JSON.stringify(await ev(`({ open: document.querySelector('#dlg-confirm').open, okText: document.querySelector('#btn-confirm-ok').textContent, step: app.scopeStep })`)));
await shot("C2b-c-cancel-noop.png");
// ---- 串台：Esc 关掉 → 打开另一台机器的对话框 → 等原请求返回 ----
await send("Input.dispatchKeyEvent", { type: "keyDown", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27 }, sid);
await send("Input.dispatchKeyEvent", { type: "keyUp", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27 }, sid);
await sleep(1200);
console.log("[after Esc] open:", await ev("document.querySelector('#dlg-confirm').open"), "| pendingAction:", await ev("JSON.stringify(app.pendingAction)"));
const OTHER = "e363efdcf085-1563";
const r2 = await ev(`(()=>{
  const card=[...document.querySelectorAll('.agent-card')].find(c=>c.getAttribute('data-agent-id')===${JSON.stringify(OTHER)});
  if(!card) return 'NOCARD';
  const b=[...card.querySelectorAll('button')].find(x=>x.textContent.trim()==='处理冲突');
  if(!b) return 'NOBTN'; b.click(); return 'clicked';})()`);
console.log("[串台] open 2nd dialog:", r2);
for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#dlg-confirm').open && document.querySelector('#confirm-choices').children.length>0")) break; }
console.log("[串台] 2nd title:", await ev("document.querySelector('#confirm-title').textContent"), "| desc:", await ev("document.querySelector('#confirm-desc').textContent.slice(0,60)"));
await shot("C2b-d-second-dialog-before.png");
// 等原 pull 返回（agent 被 STOP，最长可能几十秒）—— 轮询观察 desc 是否被改写
let crossed = null;
for (let i = 0; i < 70; i++) {
  await sleep(1000);
  const s = await ev(`({ open: document.querySelector('#dlg-confirm').open, title: document.querySelector('#confirm-title').textContent, desc: document.querySelector('#confirm-desc').textContent, kind: app.pendingAction && app.pendingAction.kind })`);
  if (s.desc && /已下发到/.test(s.desc)) { crossed = s; break; }
  if (i % 10 === 0) console.log("  ..waiting", i, "s desc:", JSON.stringify(String(s.desc).slice(0, 50)));
}
console.log("[串台] RESULT:", JSON.stringify(crossed));
await shot("C2b-e-second-dialog-after.png");
process.exit(0);
