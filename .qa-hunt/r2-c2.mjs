setTimeout(() => process.exit(9), 55_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Network.enable", {}, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
console.log("loggedIn:", await login(ev));
await sleep(800);

const TARGET = process.argv[2] || "b6675d68ddf7";
// 找目标机器卡片（容器 hostname = b6675d68ddf7，agentId = b6675d68ddf7-7920）
const idx = await ev(`(()=>{const cs=[...document.querySelectorAll('.agent-card')];return cs.findIndex(c=>c.getAttribute('data-agent-id').startsWith(${JSON.stringify(TARGET)}))})()`);
console.log("[pre] target card index:", idx);
// 点「下发」
await ev(`document.querySelectorAll('.agent-card')[${idx}].querySelector('.agent-actions button:nth-child(2)')`); // noop probe
const opened = await ev(`(()=>{
  const card=[...document.querySelectorAll('.agent-card')].find(c=>c.getAttribute('data-agent-id').startsWith(${JSON.stringify(TARGET)}));
  if(!card) return 'NOCARD';
  const btns=[...card.querySelectorAll('.agent-actions button')];
  const d=btns.find(b=>b.textContent.trim()==='下发');
  if(!d) return 'NODISPATCH:'+btns.map(b=>b.textContent.trim()).join('/');
  d.click(); return 'clicked';})()`);
console.log("[step] dispatch click:", opened);
for (let i = 0; i < 30; i++) { await sleep(300); if (await ev("document.querySelector('#dlg-confirm').open && document.querySelector('#confirm-choices').children.length > 0")) break; }
console.log("[pre] dlg open:", await ev("document.querySelector('#dlg-confirm').open"), "| kind:", await ev("app.pendingAction && app.pendingAction.kind"), "| step:", await ev("app.scopeStep"));
const cb = await ev("document.querySelector('#confirm-choices').children.length");
console.log("[pre] choices:", cb);
// 勾第一个适配器
await ev(`(()=>{const i=document.querySelector('#confirm-choices input[type=checkbox]');if(!i)return 'NOBOX';i.checked=true;i.dispatchEvent(new Event('change',{bubbles:true}));return 'checked'})()`);
await sleep(600);
console.log("[pre] ok btn:", await ev("document.querySelector('#btn-confirm-ok').textContent"), "| disabled:", await ev("document.querySelector('#btn-confirm-ok').disabled"));
await shot("C2-a-choose.png");
// 确认下发 → 进入 wait 态（后端真慢，因为 agent 被 STOP）
await ev("document.querySelector('#btn-confirm-ok').click()");
await sleep(2000);
const w = await ev(`({
  open: document.querySelector('#dlg-confirm').open,
  okText: document.querySelector('#btn-confirm-ok').textContent,
  okDisabled: document.querySelector('#btn-confirm-ok').disabled,
  cancelDisabled: document.querySelector('#btn-confirm-cancel').disabled,
  backHidden: document.querySelector('#btn-confirm-back').classList.contains('hidden'),
  step: app.scopeStep, kind: app.pendingAction && app.pendingAction.kind,
  desc: document.querySelector('#confirm-desc').textContent,
  title: document.querySelector('#confirm-title').textContent
})`);
console.log("[wait] state:", JSON.stringify(w));
await shot("C2-b-waiting-real-slow-backend.png");
// 真实鼠标点「取消」
const bx = await ev("(()=>{const r=document.querySelector('#btn-confirm-cancel').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
await send("Input.dispatchMouseEvent", { type: "mousePressed", x: bx.x, y: bx.y, button: "left", clickCount: 1 }, sid);
await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: bx.x, y: bx.y, button: "left", clickCount: 1 }, sid);
await sleep(1500);
console.log("[cancel-click] ->", JSON.stringify(await ev(`({ open: document.querySelector('#dlg-confirm').open, okText: document.querySelector('#btn-confirm-ok').textContent, step: app.scopeStep })`)));
await shot("C2-c-after-cancel-click.png");
process.exit(0);
