import { attach, nav, ev, sleep, shot, waitFor, LOG } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);
// open 收取 dialog on a machine that has push>0, or force via confirmCollect
const r = await ev(sid, `(()=>{const a=app.agents.find(a=>!a.stale);if(!a)return 'none';confirmCollect(a);return a.agentId})()`);
await waitFor(sid,"document.querySelector('#confirm-choices').children.length>0",15000);
// tick a checkbox + expand a details
const s1 = await ev(sid, `(()=>{const i=[...document.querySelectorAll('#confirm-choices input[data-adapter]')].find(i=>!i.disabled);
  if(i){i.checked=true;i.dispatchEvent(new Event('change',{bubbles:true}))}
  const d=document.querySelector('#confirm-choices details'); if(d) d.open=true;
  return JSON.stringify({checked:i&&i.dataset.adapter, detailsOpen:document.querySelectorAll('#confirm-choices details[open]').length})})()`);
L("before refresh:", s1, "| ok:", await ev(sid,"JSON.stringify({t:document.querySelector('#btn-confirm-ok').textContent,d:document.querySelector('#btn-confirm-ok').disabled})"));
await ev(sid,"refresh()"); await sleep(3500);
const s2 = await ev(sid, `JSON.stringify({checked:[...document.querySelectorAll('#confirm-choices input[data-adapter]')].filter(i=>i.checked).map(i=>i.dataset.adapter),
  detailsOpen:document.querySelectorAll('#confirm-choices details[open]').length,
  dlgOpen:document.querySelector('#dlg-confirm').open, title:document.querySelector('#confirm-title').textContent})`);
L("after refresh:", s2);
L("ok after:", await ev(sid,"JSON.stringify({t:document.querySelector('#btn-confirm-ok').textContent,d:document.querySelector('#btn-confirm-ok').disabled})"));
// also verify a real dispatch dialog keeps its checkbox
await ev(sid,"document.querySelector('#dlg-confirm').close('cancel')"); await sleep(500);
const r2 = await ev(sid, `(()=>{const a=app.agents.find(a=>!a.stale);confirmDispatch(a);return a.agentId})()`);
await waitFor(sid,"document.querySelector('#confirm-choices').children.length>0",15000);
const b1 = await ev(sid, `(()=>{const i=[...document.querySelectorAll('#confirm-choices input[data-adapter]')][0]; if(i&&!i.disabled){i.checked=true;i.dispatchEvent(new Event('change',{bubbles:true}))} return JSON.stringify({checked:[...document.querySelectorAll('#confirm-choices input[data-adapter]')].filter(i=>i.checked).map(i=>i.dataset.adapter)})})()`);
L("dispatch dlg before refresh:", b1);
await ev(sid,"refresh()"); await sleep(3500);
L("dispatch dlg after refresh:", await ev(sid,`JSON.stringify({checked:[...document.querySelectorAll('#confirm-choices input[data-adapter]')].filter(i=>i.checked).map(i=>i.dataset.adapter),open:document.querySelector('#dlg-confirm').open})`));
L("ERRS:", JSON.stringify(LOG.err.slice(0,3)));
process.exit(0);
