process.on("unhandledRejection", e => console.log("[suppressed]", String(e).slice(0,50)));
setTimeout(() => { console.log("[GUARD] exit"); process.exit(9); }, 115_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
console.log("loggedIn:", await login(ev));
await sleep(500);
const ID = "b6675d68ddf7-8e7b";
// PHASE A: agent 还在线，先打开对话框（choices 就位）
const r = await ev(`(()=>{const card=[...document.querySelectorAll('.agent-card')].find(c=>c.getAttribute('data-agent-id')===${JSON.stringify(ID)});
  const b=[...card.querySelectorAll('button')].find(x=>x.textContent.trim()==='下发'); if(!b) return 'NOBTN:'+[...card.querySelectorAll('.agent-actions button')].map(x=>x.textContent.trim()).join('/'); b.click(); return 'clicked';})()`);
console.log("[A] dispatch:", r);
for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#dlg-confirm').open && document.querySelector('#confirm-choices').children.length>0")) break; }
console.log("[A] kind:", await ev("app.pendingAction&&app.pendingAction.kind"), "| choices:", await ev("document.querySelector('#confirm-choices').children.length"));
// 勾选
const ck = await ev(`(()=>{const i=[...document.querySelectorAll('#confirm-choices input[type=checkbox]')].find(x=>!x.disabled);if(!i)return 'NOBOX';i.checked=true;i.dispatchEvent(new Event('change',{bubbles:true}));return i.dataset.adapter;})()`);
await sleep(500);
console.log("[A] checked:", ck, "| ok:", await ev("document.querySelector('#btn-confirm-ok').textContent"), await ev("document.querySelector('#btn-confirm-ok').disabled"));
console.log("[A] PAUSE-NOW");
// 在此停住：等待 shell 把 agent STOP（轮询外部文件标志）
for (let i = 0; i < 60; i++) {
  const f = await ev("window.__go === true");
  if (f) break;
  await sleep(1000);
}
console.log("[B] go! clicking 确认下发 (agent is STOPped now)");
await ev("document.querySelector('#btn-confirm-ok').click()");
await sleep(3000);
const wait = await ev(`({open:document.querySelector('#dlg-confirm').open, okText:document.querySelector('#btn-confirm-ok').textContent, okDisabled:document.querySelector('#btn-confirm-ok').disabled, cancelDisabled:document.querySelector('#btn-confirm-cancel').disabled, backHidden:document.querySelector('#btn-confirm-back').classList.contains('hidden'), step:app.scopeStep, title:document.querySelector('#confirm-title').textContent})`);
console.log("[WAIT]", JSON.stringify(wait));
await shot("C2f-b-waiting-REAL.png");
if (!wait.open || !wait.cancelDisabled) { console.log("[ABORT] not in wait"); process.exit(0); }
// 真实鼠标点取消
const bx = await ev("(()=>{const r=document.querySelector('#btn-confirm-cancel').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
await send("Input.dispatchMouseEvent", { type: "mousePressed", x: bx.x, y: bx.y, button: "left", clickCount: 1 }, sid);
await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: bx.x, y: bx.y, button: "left", clickCount: 1 }, sid);
await sleep(1500);
console.log("[after CANCEL click]", JSON.stringify(await ev(`({open:document.querySelector('#dlg-confirm').open, okText:document.querySelector('#btn-confirm-ok').textContent, step:app.scopeStep})`)));
await shot("C2f-c-cancel-noop.png");
// Esc
await send("Input.dispatchKeyEvent", { type: "keyDown", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27 }, sid);
await send("Input.dispatchKeyEvent", { type: "keyUp", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27 }, sid);
await sleep(1000);
console.log("[after ESC] open:", await ev("document.querySelector('#dlg-confirm').open"), "| pendingAction:", await ev("JSON.stringify(app.pendingAction)"));
// 打开 54d0c3373428-bc77 的查看对话框（标题不同）
const r2 = await ev(`(()=>{const card=[...document.querySelectorAll('.agent-card')].find(c=>c.getAttribute('data-agent-id')==='54d0c3373428-bc77');
  const b=[...card.querySelectorAll('button')].find(x=>x.textContent.trim()==='查看'); if(!b) return 'NOBTN'; b.click(); return 'view';})()`);
console.log("[2nd]", r2);
for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#dlg-view').open")) break; }
// 观察 view-title（孤儿回调不应改它，但 confirm-desc 会变 —— dlg-view 之下 confirm desc 不可见，所以 ALSO 记录 confirm-desc）
await ev(`(()=>{window.__log=[];['#view-title','#confirm-desc','#confirm-title'].forEach(sel=>{const el=document.querySelector(sel);if(!el)return;
  new MutationObserver(()=>window.__log.push(sel+'='+el.textContent.slice(0,60))).observe(el,{childList:true,characterData:true,subtree:true});});return 1})()`);
console.log("[2nd] view-title:", await ev("document.querySelector('#view-title').textContent"));
await shot("C2f-d-second-dialog-open.png");
console.log("PHASE-A-DONE — shell: kill -CONT 1005 now");
// 轮询 __log 最多 60s（agent CONT 后孤儿返回）
for (let i = 0; i < 60; i++) {
  await sleep(1000);
  const f = await ev("window.__go2 === true");
  if (f) break;
}
await sleep(1000);
const log = await ev("window.__log");
console.log("[LOG after orphan returns]", JSON.stringify(log));
console.log("[now confirm-desc]:", JSON.stringify(String(await ev("document.querySelector('#confirm-desc').textContent")).slice(0,70)));
console.log("[dlg-view still open]:", await ev("document.querySelector('#dlg-view').open"));
await shot("C2f-e-after-orphan.png");
process.exit(0);
