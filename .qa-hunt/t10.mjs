import { attach, nav, ev, sleep, shot, waitFor, LOG } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);
const r = await ev(sid, `(()=>{const c=[...document.querySelectorAll('.agent-card')].find(c=>[...c.querySelectorAll('button')].some(b=>b.textContent.trim()==='收取'));if(!c)return 'none';[...c.querySelectorAll('button')].find(b=>b.textContent.trim()==='收取').click();return c.dataset.agentId})()`);
L("collect target:", r);
if (r==='none'){L("no collect btn");process.exit(0);}
await waitFor(sid,"document.querySelector('#confirm-choices').children.length>0",15000);
L("title:", await ev(sid,"document.querySelector('#confirm-title').textContent"));
L("desc:", await ev(sid,"document.querySelector('#confirm-desc').textContent"));
L("note:", await ev(sid,"document.querySelector('#confirm-note').textContent"));
L("adapters:", await ev(sid,"JSON.stringify([...document.querySelectorAll('#confirm-choices input[data-adapter]')].map(i=>[i.dataset.adapter,i.checked,i.disabled]))"));
L("open details in choices:", await ev(sid,"document.querySelectorAll('#confirm-choices details[open]').length"));
L("ok:", await ev(sid,"JSON.stringify({t:document.querySelector('#btn-confirm-ok').textContent,d:document.querySelector('#btn-confirm-ok').disabled})"));
// count expanded diff drawers
const d1 = await ev(sid,"document.querySelectorAll('#confirm-choices details[open]').length");
// fire a background refresh while dialog is open (30s poll does this)
await ev(sid,"refresh()");
await sleep(3500);
const d2 = await ev(sid,"document.querySelectorAll('#confirm-choices details[open]').length");
L("details open before refresh:", d1, "after refresh():", d2);
L("dialog still open:", await ev(sid,"document.querySelector('#dlg-confirm').open"));
L("desc after refresh:", await ev(sid,"document.querySelector('#confirm-desc').textContent"));
L("adapters after refresh:", await ev(sid,"JSON.stringify([...document.querySelectorAll('#confirm-choices input[data-adapter]')].map(i=>[i.dataset.adapter,i.checked]))"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/18-collect-dlg.png");
// uncheck all -> click OK
await ev(sid,"[...document.querySelectorAll('#confirm-choices input[data-adapter]')].forEach(i=>{i.checked=false;i.dispatchEvent(new Event('change',{bubbles:true}))})");
await sleep(400);
L("after uncheck ok.disabled:", await ev(sid,"document.querySelector('#btn-confirm-ok').disabled"));
await ev(sid,"document.querySelector('#btn-confirm-ok').click()");
await sleep(800);
L("after OK dlg open:", await ev(sid,"document.querySelector('#dlg-confirm').open"), "toast:", await ev(sid,"document.querySelector('#toast').textContent"));
L("ERRS:", JSON.stringify(LOG.err.slice(0,5)));
process.exit(0);
