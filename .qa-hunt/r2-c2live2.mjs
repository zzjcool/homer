process.on("unhandledRejection", e => console.log("[suppressed]", String(e).slice(0,50)));
setTimeout(() => { console.log("[GUARD] exit"); process.exit(9); }, 100_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
console.log("loggedIn:", await login(ev));
await sleep(600);
const ID = "b6675d68ddf7-6dd5";
const r = await ev(`(()=>{const card=[...document.querySelectorAll('.agent-card')].find(c=>c.getAttribute('data-agent-id')===${JSON.stringify(ID)});
  if(!card) return 'NOCARD';
  const b=[...card.querySelectorAll('button')].find(x=>x.textContent.trim()==='下发');
  if(!b) return 'NOBTN'; b.click(); return 'clicked';})()`);
console.log("[step] dispatch:", r);
for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#dlg-confirm').open && document.querySelector('#confirm-choices').children.length>0")) break; }
console.log("[pre] kind:", await ev("app.pendingAction&&app.pendingAction.kind"), "| title:", await ev("document.querySelector('#confirm-title').textContent"));
const ck = await ev(`(()=>{const i=[...document.querySelectorAll('#confirm-choices input[type=checkbox]')].find(x=>!x.disabled);if(!i)return 'NOBOX';i.checked=true;i.dispatchEvent(new Event('change',{bubbles:true}));return i.dataset.adapter;})()`);
await sleep(500);
console.log("[pre] checked:", ck, "| ok:", await ev("document.querySelector('#btn-confirm-ok').textContent"), "disabled:", await ev("document.querySelector('#btn-confirm-ok').disabled"));
await ev("document.querySelector('#btn-confirm-ok').click()");
await sleep(2500);
console.log("[WAIT]", JSON.stringify(await ev(`({open:document.querySelector('#dlg-confirm').open, okText:document.querySelector('#btn-confirm-ok').textContent, cancelDisabled:document.querySelector('#btn-confirm-cancel').disabled, step:app.scopeStep, title:document.querySelector('#confirm-title').textContent})`)));
await shot("C2live-b-waiting-REAL.png");
// 真实鼠标点取消
const bx = await ev("(()=>{const r=document.querySelector('#btn-confirm-cancel').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
await send("Input.dispatchMouseEvent", { type: "mousePressed", x: bx.x, y: bx.y, button: "left", clickCount: 1 }, sid);
await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: bx.x, y: bx.y, button: "left", clickCount: 1 }, sid);
await sleep(1500);
console.log("[after CANCEL click]", JSON.stringify(await ev(`({open:document.querySelector('#dlg-confirm').open, okText:document.querySelector('#btn-confirm-ok').textContent, step:app.scopeStep})`)));
await shot("C2live-c-cancel-noop.png");
// Esc 关掉
await send("Input.dispatchKeyEvent", { type: "keyDown", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27 }, sid);
await send("Input.dispatchKeyEvent", { type: "keyUp", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27 }, sid);
await sleep(1200);
console.log("[after ESC] open:", await ev("document.querySelector('#dlg-confirm').open"), "| pendingAction:", await ev("JSON.stringify(app.pendingAction)"));
// 打开另一台机器的下发对话框
const r2 = await ev(`(()=>{const card=[...document.querySelectorAll('.agent-card')].find(c=>c.getAttribute('data-agent-id')==='54d0c3373428-bc77');
  const b=[...card.querySelectorAll('button')].find(x=>x.textContent.trim()==='下发');
  if(!b) return 'NOBTN'; b.click(); return 'clicked';})()`);
console.log("[2nd]", r2);
for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#dlg-confirm').open && document.querySelector('#confirm-choices').children.length>0")) break; }
console.log("[2nd] title:", await ev("document.querySelector('#confirm-title').textContent"));
await shot("C2live-d-second-dialog.png");
// 页面侧记录 desc 变化
await ev(`(()=>{window.__desc=[];const box=document.querySelector('#confirm-desc');
  new MutationObserver(()=>window.__desc.push(box.textContent.slice(0,60))).observe(box,{childList:true,characterData:true,subtree:true});return 1})()`);
console.log("[NOW] sending SIGCONT to agent 858 (orphan pull will return)");
console.log("CONT-SENT");
process.exit(0);
