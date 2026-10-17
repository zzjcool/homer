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
const ID = "b6675d68ddf7-8e7b";
const r = await ev(`(()=>{const card=[...document.querySelectorAll('.agent-card')].find(c=>c.getAttribute('data-agent-id')===${JSON.stringify(ID)});
  if(!card) return 'NOCARD';
  const b=[...card.querySelectorAll('button')].find(x=>x.textContent.trim()==='下发');
  if(!b) return 'NOBTN:'+[...card.querySelectorAll('.agent-actions button')].map(x=>x.textContent.trim()).join('/'); b.click(); return 'clicked';})()`);
console.log("[step] dispatch:", r);
for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#dlg-confirm').open && document.querySelector('#confirm-choices').children.length>0")) break; }
console.log("[pre] kind:", await ev("app.pendingAction&&app.pendingAction.kind"), "| title:", await ev("document.querySelector('#confirm-title').textContent"), "| ok:", await ev("document.querySelector('#btn-confirm-ok').textContent"), await ev("document.querySelector('#btn-confirm-ok').disabled"));
const ck = await ev(`(()=>{const i=[...document.querySelectorAll('#confirm-choices input[type=checkbox]')].find(x=>!x.disabled);if(!i)return 'NOBOX:'+document.querySelector('#confirm-choices').textContent.slice(0,80);i.checked=true;i.dispatchEvent(new Event('change',{bubbles:true}));return i.dataset.adapter;})()`);
await sleep(500);
console.log("[pre] checked:", ck, "| ok:", await ev("document.querySelector('#btn-confirm-ok').textContent"), await ev("document.querySelector('#btn-confirm-ok').disabled"));
await shot("C2live-a-choose.png");
await ev("document.querySelector('#btn-confirm-ok').click()");
await sleep(3000);
console.log("[WAIT]", JSON.stringify(await ev(`({open:document.querySelector('#dlg-confirm').open, okText:document.querySelector('#btn-confirm-ok').textContent, okDisabled:document.querySelector('#btn-confirm-ok').disabled, cancelDisabled:document.querySelector('#btn-confirm-cancel').disabled, backHidden:document.querySelector('#btn-confirm-back').classList.contains('hidden'), step:app.scopeStep, title:document.querySelector('#confirm-title').textContent, desc:String(document.querySelector('#confirm-desc').textContent).slice(0,50)})`)));
await shot("C2live-b-waiting-REAL.png");
const bx = await ev("(()=>{const r=document.querySelector('#btn-confirm-cancel').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
await send("Input.dispatchMouseEvent", { type: "mousePressed", x: bx.x, y: bx.y, button: "left", clickCount: 1 }, sid);
await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: bx.x, y: bx.y, button: "left", clickCount: 1 }, sid);
await sleep(1500);
console.log("[after CANCEL click]", JSON.stringify(await ev(`({open:document.querySelector('#dlg-confirm').open, okText:document.querySelector('#btn-confirm-ok').textContent, step:app.scopeStep})`)));
await shot("C2live-c-cancel-noop.png");
await send("Input.dispatchKeyEvent", { type: "keyDown", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27 }, sid);
await send("Input.dispatchKeyEvent", { type: "keyUp", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27 }, sid);
await sleep(1200);
console.log("[after ESC] open:", await ev("document.querySelector('#dlg-confirm').open"), "| pendingAction:", await ev("JSON.stringify(app.pendingAction)"));
// 打开另一台机器的下发对话框（54d0c3373428-bc77 无冲突有下发按钮？——用它的「查看」更稳：标题肯定不同）
const r2 = await ev(`(()=>{const card=[...document.querySelectorAll('.agent-card')].find(c=>c.getAttribute('data-agent-id')==='54d0c3373428-bc77');
  const b=[...card.querySelectorAll('button')].find(x=>['下发','查看'].includes(x.textContent.trim()));
  if(!b) return 'NOBTN'; b.click(); return b.textContent.trim();})()`);
console.log("[2nd] opened:", r2);
for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#dlg-confirm').open && (document.querySelector('#confirm-choices').children.length>0 || document.querySelector('#view-body'))")) break; }
// 打开后装 MutationObserver 记录 confirm-desc 的变化
await ev(`(()=>{window.__log=[];['#confirm-desc','#view-title'].forEach(sel=>{const el=document.querySelector(sel);if(!el)return;
  new MutationObserver(()=>window.__log.push(sel+'='+el.textContent.slice(0,50))).observe(el,{childList:true,characterData:true,subtree:true});});
  window.__log.push('BASE-title='+document.querySelector('#confirm-title').textContent);return 1})()`);
console.log("[2nd] title:", await ev("document.querySelector('#confirm-title').textContent"), "| desc/view:", String(await ev("document.querySelector('#confirm-desc').textContent")).slice(0,60));
await shot("C2live-d-second-dialog-open.png");
console.log("PHASE1-DONE");
process.exit(0);
