import { attach, nav, ev, sleep, shot, waitFor, LOG, cdp } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);
await ev(sid, `(()=>{const of=window.fetch.bind(window);window.__release=null;
  window.fetch=(u,o)=>{const url=String(u);if(url.includes('/pull')){window.__pullCalls=(window.__pullCalls||0)+1;
    return new Promise((res,rej)=>{window.__release=()=>of(u,o).then(res,rej)})}return of(u,o)};return 'p'})()`);
// dispatch A hung
await ev(sid, `(()=>{const a=app.agents.find(a=>!a.stale)||app.agents[0];
  app.scopeDirection='dispatch';app.scopeStep='choose';app.scopeAgent=a;app.scopeChoices=[{id:'pi',enabled:true}];app.scopeKeys=[];
  app.pendingAction={kind:'dispatch-one',agentId:a.agentId,name:(a.hostname||a.agentId)};
  document.querySelector('#dlg-confirm').showModal();
  document.querySelector('#confirm-choices').replaceChildren();
  const i=document.createElement('input');i.type='checkbox';i.dataset.adapter='pi';i.checked=true;document.querySelector('#confirm-choices').appendChild(i);
  document.querySelector('#btn-confirm-ok').disabled=false;document.querySelector('#btn-confirm-ok').textContent='确认下发';
  return a.agentId})()`);
await ev(sid,"document.querySelector('#btn-confirm-ok').click()");
await sleep(1200);
// Esc to "cancel"
await cdp("Input.dispatchKeyEvent",{type:"keyDown",key:"Escape",code:"Escape",windowsVirtualKeyCode:27,nativeVirtualKeyCode:27},sid);
await cdp("Input.dispatchKeyEvent",{type:"keyUp",key:"Escape",code:"Escape",windowsVirtualKeyCode:27,nativeVirtualKeyCode:27},sid);
await sleep(700);
L("after Esc: open:", await ev(sid,"document.querySelector('#dlg-confirm').open"), "pending:", await ev(sid,"JSON.stringify(app.pendingAction)"));
// NOW open a DIFFERENT dialog: 处理冲突 on another machine (or 收取)
const r2 = await ev(sid, `(()=>{const c=[...document.querySelectorAll('.agent-card')].find(c=>[...c.querySelectorAll('button')].some(b=>b.textContent.trim()==='收取'));
  if(c){[...c.querySelectorAll('button')].find(b=>b.textContent.trim()==='收取').click();return 'collect:'+c.dataset.agentId}
  const c2=[...document.querySelectorAll('.agent-card')].find(c=>[...c.querySelectorAll('button')].some(b=>b.textContent.trim()==='处理冲突'));
  if(!c2) return 'none';[...c2.querySelectorAll('button')].find(b=>b.textContent.trim()==='处理冲突').click();return 'resolve:'+c2.dataset.agentId})()`);
L("opened second dialog:", r2);
await waitFor(sid,"document.querySelector('#confirm-choices').children.length>0",12000);
const before = await ev(sid,"document.querySelector('#confirm-desc').textContent");
L("second dialog desc BEFORE orphan completes:", before);
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/C7-second-dialog-before.png");
// release the orphaned dispatch -> it will write into the now-open second dialog
await ev(sid,"(()=>{const r=window.__release;if(r){r();return 'released'}return 'noop'})()");
await sleep(5000);
L("second dialog desc AFTER orphan completes:", await ev(sid,"document.querySelector('#confirm-desc').textContent"));
L("second dialog note AFTER:", await ev(sid,"document.querySelector('#confirm-note').textContent"));
L("pendingAction now:", await ev(sid,"JSON.stringify(app.pendingAction)"));
L("second dialog still open:", await ev(sid,"document.querySelector('#dlg-confirm').open"));
L("ok text/disabled:", await ev(sid,"JSON.stringify({t:document.querySelector('#btn-confirm-ok').textContent,d:document.querySelector('#btn-confirm-ok').disabled})"));
L("title:", await ev(sid,"document.querySelector('#confirm-title').textContent"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/C7-second-dialog-AFTER-orphan.png");
L("ERRS:", JSON.stringify(LOG.err.slice(0,5)));
process.exit(0);
