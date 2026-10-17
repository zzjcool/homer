import { attach, nav, ev, sleep, shot, waitFor, LOG } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);
const card = async (sel) => ev(sid, `(()=>{const c=[...document.querySelectorAll('.agent-card')].find(c=>${sel});return c?c.dataset.agentId:null})()`);

// ==== T2: dispatch on machine with pull>0 (54d0c3373428-62af)
const id = await card("c.textContent.includes('待下发')");
L("T2 target:", id);
await ev(sid, `(()=>{const c=[...document.querySelectorAll('.agent-card')].find(c=>c.textContent.includes('待下发'));[...c.querySelectorAll('button')].find(b=>b.textContent.trim()==='下发').click()})()`);
await waitFor(sid,"document.querySelector('#confirm-choices').children.length>0",12000);
L("T2 dlg open:", await ev(sid,"document.querySelector('#dlg-confirm').open"));
L("T2 desc:", await ev(sid,"document.querySelector('#confirm-desc').textContent"));
L("T2 adapters:", await ev(sid,"JSON.stringify([...document.querySelectorAll('#confirm-choices input[data-adapter]')].map(i=>[i.dataset.adapter,i.checked,i.disabled]))"));
L("T2 ok:", await ev(sid,"JSON.stringify({t:document.querySelector('#btn-confirm-ok').textContent,d:document.querySelector('#btn-confirm-ok').disabled})"));
L("T2 pendingAction:", await ev(sid,"JSON.stringify({k:app.pendingAction&&app.pendingAction.kind,step:app.scopeStep,dir:app.scopeDirection})"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/02-dispatch-dlg.png");

// ==== T3: 全不选 then click OK -> silent no-op?
await ev(sid,"[...document.querySelectorAll('#confirm-choices input[data-adapter]')].forEach(i=>{i.checked=false;i.dispatchEvent(new Event('change',{bubbles:true}))})");
await sleep(400);
L("T3 after uncheck all - ok disabled:", await ev(sid,"document.querySelector('#btn-confirm-ok').disabled"));
await ev(sid,"document.querySelector('#btn-confirm-ok').click()");
await sleep(800);
L("T3 after click OK - dlg still open:", await ev(sid,"document.querySelector('#dlg-confirm').open"));
L("T3 toast text:", await ev(sid,"document.querySelector('#toast').textContent"));
L("T3 toast visible:", await ev(sid,"!document.querySelector('#toast').classList.contains('hidden')"));
L("T3 dialogs:", await ev(sid,"JSON.stringify(window.__dlg)"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/04-ok-silent-noop.png");
L("ERRS:", JSON.stringify(LOG.err), JSON.stringify(LOG.console));
process.exit(0);
