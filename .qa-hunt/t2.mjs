import { attach, nav, ev, sleep, shot, waitFor } from "./lib-b.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);

// --- T1: keys-fold-hint static?
L("T1 keys-fold-hint:", await ev(sid,"document.querySelector('#keys-fold-hint').textContent"));
await ev(sid,"document.querySelector('#keys-fold').open=true");
await sleep(500);
await waitFor(sid,"document.querySelector('#key-body').children.length>0",6000);
L("T1 key-body:", await ev(sid,"document.querySelector('#key-body').textContent.slice(0,200)"));
L("T1 keys-fold-hint after load:", await ev(sid,"document.querySelector('#keys-fold-hint').textContent"));

// --- T2: dispatch dialog on 新机器
await ev(sid, `(()=>{const cards=[...document.querySelectorAll('.agent-card')];const c=cards.find(c=>c.textContent.includes('等待下发'));const b=[...c.querySelectorAll('button')].find(b=>b.textContent.trim()==='下发');b.click();return c.dataset.agentId})()`);
await waitFor(sid,"document.querySelector('#dlg-confirm').open",6000);
await waitFor(sid,"document.querySelector('#confirm-choices').children.length>0",10000);
L("T2 dlg open:", await ev(sid,"document.querySelector('#dlg-confirm').open"));
L("T2 desc:", await ev(sid,"document.querySelector('#confirm-desc').textContent"));
L("T2 pendingAction:", await ev(sid,"JSON.stringify({kind:app.pendingAction&&app.pendingAction.kind,step:app.scopeStep,dir:app.scopeDirection})"));
L("T2 adapter checkboxes:", await ev(sid,"JSON.stringify([...document.querySelectorAll('#confirm-choices input[data-adapter]')].map(i=>[i.dataset.adapter,i.checked,i.disabled]))"));
L("T2 ok btn:", await ev(sid,"JSON.stringify({t:btn_confirm_ok.textContent,d:btn_confirm_ok.disabled})"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/02-dispatch-dlg.png");
await ev(sid,"document.querySelector('#dlg-confirm').close('cancel')");
process.exit(0);
