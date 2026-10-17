import { attach, nav, ev, sleep, shot, waitFor, LOG, cdp } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);
// pause any /pull?confirm=true request to simulate a hung backend
await cdp("Fetch.enable",{patterns:[{urlPattern:"*pull*",requestStage:"Request"}]},sid);
let paused=[];
cdp("Fetch.enable",{},sid);
// open 下发 on any card that has it
const r = await ev(sid, `(()=>{const c=[...document.querySelectorAll('.agent-card')].find(c=>[...c.querySelectorAll('button')].some(b=>b.textContent.trim()==='下发'));if(!c)return 'none';[...c.querySelectorAll('button')].find(b=>b.textContent.trim()==='下发').click();return c.dataset.agentId})()`);
L("target:", r);
if (r === 'none') { L("no dispatch button; skip"); process.exit(0); }
await waitFor(sid,"document.querySelector('#confirm-choices').children.length>0",12000);
L("adapters:", await ev(sid,"JSON.stringify([...document.querySelectorAll('#confirm-choices input[data-adapter]')].map(i=>[i.dataset.adapter,i.checked,i.disabled]))"));
L("ok:", await ev(sid,"JSON.stringify({t:document.querySelector('#btn-confirm-ok').textContent,d:document.querySelector('#btn-confirm-ok').disabled})"));
// click OK once
await ev(sid, "document.querySelector('#btn-confirm-ok').click()");
await sleep(1500);
L("after OK#1 dlg open:", await ev(sid,"document.querySelector('#dlg-confirm').open"));
L("after OK#1 desc:", await ev(sid,"document.querySelector('#confirm-desc').textContent"));
L("after OK#1 okbtn:", await ev(sid,"JSON.stringify({t:document.querySelector('#btn-confirm-ok').textContent,d:document.querySelector('#btn-confirm-ok').disabled})"));
L("after OK#1 cancel disabled:", await ev(sid,"document.querySelector('#btn-confirm-cancel').disabled"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/15-dispatch-wait-0s.png");
await sleep(9000);
L("t+10s okbtn:", await ev(sid,"JSON.stringify({t:document.querySelector('#btn-confirm-ok').textContent,d:document.querySelector('#btn-confirm-ok').disabled})"));
L("t+10s desc:", await ev(sid,"document.querySelector('#confirm-desc').textContent"));
L("t+10s cancel disabled:", await ev(sid,"document.querySelector('#btn-confirm-cancel').disabled"));
L("t+10s back btn:", await ev(sid,"JSON.stringify({hidden:document.querySelector('#btn-confirm-back').classList.contains('hidden'),disabled:document.querySelector('#btn-confirm-back').disabled})"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/16-dispatch-wait-10s.png");
// try to cancel
await ev(sid,"document.querySelector('#btn-confirm-cancel').click()");
await sleep(800);
L("after cancel click dlg open:", await ev(sid,"document.querySelector('#dlg-confirm').open"));
L("after cancel click okbtn:", await ev(sid,"document.querySelector('#btn-confirm-ok').textContent"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/17-dispatch-cancel-attempt.png");
L("ERRS:", JSON.stringify(LOG.err.slice(0,5)));
process.exit(0);
