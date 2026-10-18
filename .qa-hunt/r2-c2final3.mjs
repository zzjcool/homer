process.on("unhandledRejection", e => console.log("[suppressed]", String(e).slice(0,50)));
setTimeout(() => { console.log("[GUARD] exit"); process.exit(9); }, 115_000).unref();
import { connect, sleep, login } from "/root/code/homer/.qa-hunt/repro2-lib.mjs";
import { execSync } from "node:child_process";
const { send, ev, sid, shot } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
console.log("loggedIn:", await login(ev));
await sleep(500);
const ID = "b6675d68ddf7-8e7b";
// PHASE A：agent 在线，先打开对话框
const r = await ev(`(()=>{const card=[...document.querySelectorAll('.agent-card')].find(c=>c.getAttribute('data-agent-id')===${JSON.stringify(ID)});
  const b=[...card.querySelectorAll('button')].find(x=>x.textContent.trim()==='下发'); if(!b) return 'NOBTN'; b.click(); return 'clicked';})()`);
console.log("[A] dispatch:", r);
for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#dlg-confirm').open && document.querySelector('#confirm-choices').children.length>0")) break; }
console.log("[A] kind:", await ev("app.pendingAction&&app.pendingAction.kind"), "| choices:", await ev("document.querySelector('#confirm-choices').children.length"));
const ck = await ev(`(()=>{const i=[...document.querySelectorAll('#confirm-choices input[type=checkbox]')].find(x=>!x.disabled);if(!i)return 'NOBOX';i.checked=true;i.dispatchEvent(new Event('change',{bubbles:true}));return i.dataset.adapter;})()`);
await sleep(500);
console.log("[A] checked:", ck, "| ok:", await ev("document.querySelector('#btn-confirm-ok').textContent"), await ev("document.querySelector('#btn-confirm-ok').disabled"));
// PHASE B: STOP agent（真实慢后端）
execSync("docker exec homer-qa-box-b sh -c 'kill -STOP 1005'");
console.log("[B] agent STOPPED; clicking 确认下发");
await ev("document.querySelector('#btn-confirm-ok').click()");
await sleep(3000);
const wait = await ev(`({open:document.querySelector('#dlg-confirm').open, okText:document.querySelector('#btn-confirm-ok').textContent, okDisabled:document.querySelector('#btn-confirm-ok').disabled, cancelDisabled:document.querySelector('#btn-confirm-cancel').disabled, backHidden:document.querySelector('#btn-confirm-back').classList.contains('hidden'), step:app.scopeStep, title:document.querySelector('#confirm-title').textContent})`);
console.log("[WAIT]", JSON.stringify(wait));
await shot("C2f-b-waiting-REAL.png");
if (!wait.open || !wait.cancelDisabled) { execSync("docker exec homer-qa-box-b sh -c 'kill -CONT 1005'"); console.log("[ABORT]"); process.exit(0); }
const bx = await ev("(()=>{const r=document.querySelector('#btn-confirm-cancel').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
await send("Input.dispatchMouseEvent", { type: "mousePressed", x: bx.x, y: bx.y, button: "left", clickCount: 1 }, sid);
await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: bx.x, y: bx.y, button: "left", clickCount: 1 }, sid);
await sleep(1500);
console.log("[after CANCEL click]", JSON.stringify(await ev(`({open:document.querySelector('#dlg-confirm').open, okText:document.querySelector('#btn-confirm-ok').textContent, step:app.scopeStep})`)));
await shot("C2f-c-cancel-noop.png");
await send("Input.dispatchKeyEvent", { type: "keyDown", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27 }, sid);
await send("Input.dispatchKeyEvent", { type: "keyUp", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27 }, sid);
await sleep(1000);
console.log("[after ESC] open:", await ev("document.querySelector('#dlg-confirm').open"), "| pendingAction:", await ev("JSON.stringify(app.pendingAction)"));
// 打开另一台机器查看对话框
const r2 = await ev(`(()=>{const card=[...document.querySelectorAll('.agent-card')].find(c=>c.getAttribute('data-agent-id')==='54d0c3373428-bc77');
  const b=[...card.querySelectorAll('button')].find(x=>x.textContent.trim()==='查看'); if(!b) return 'NOBTN'; b.click(); return 'view';})()`);
console.log("[2nd]", r2);
for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#dlg-view').open")) break; }
await ev(`(()=>{window.__log=[];['#view-title','#confirm-desc','#confirm-title'].forEach(sel=>{const el=document.querySelector(sel);if(!el)return;
  new MutationObserver(()=>window.__log.push(sel+'='+el.textContent.slice(0,60))).observe(el,{childList:true,characterData:true,subtree:true});});return 1})()`);
console.log("[2nd] view-title:", await ev("document.querySelector('#view-title').textContent"));
await shot("C2f-d-second-dialog-open.png");
// CONT 让孤儿返回
execSync("docker exec homer-qa-box-b sh -c 'kill -CONT 1005'");
console.log("[C] agent CONTinued — waiting for orphan fetch to return");
let log = null;
for (let i = 0; i < 70; i++) {
  await sleep(1000);
  const l = await Promise.race([ev("window.__log && window.__log.length ? JSON.stringify(window.__log) : ''").catch(() => ''), new Promise(res => setTimeout(() => res('BLOCKED'), 6000))]);
  if (l && l !== '' && l !== 'BLOCKED') { log = l; break; }
  if (i % 10 === 0) console.log("  ..waiting", i, "s");
}
console.log("[LOG]", log || "(no mutation observed in 70s)");
console.log("[confirm-desc now]:", JSON.stringify(String(await ev("document.querySelector('#confirm-desc').textContent")).slice(0,80)));
console.log("[dlg-view open]:", await ev("document.querySelector('#dlg-view').open"), "| [dlg-confirm open]:", await ev("document.querySelector('#dlg-confirm').open"));
await shot("C2f-e-after-orphan.png");
process.exit(0);
