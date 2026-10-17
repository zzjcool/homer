import { attach, nav, ev, sleep, shot, waitFor, LOG, cdp } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);
// controllable hang of /pull
await ev(sid, `(()=>{const of=window.fetch.bind(window);
  window.__release=null;
  window.fetch=(u,o)=>{const url=String(u);
    if(url.includes('/pull')) { window.__pullCalls=(window.__pullCalls||0)+1;
      return new Promise((res,rej)=>{ window.__release=()=>of(u,o).then(res,rej); }); }
    return of(u,o)};return 'patched'})()`);
await ev(sid, `(()=>{const a=app.agents.find(a=>!a.stale)||app.agents[0];
  app.scopeDirection='dispatch';app.scopeStep='choose';app.scopeAgent=a;app.scopeChoices=[{id:'pi',enabled:true}];app.scopeKeys=[];
  app.pendingAction={kind:'dispatch-one',agentId:a.agentId,name:(a.hostname||a.agentId)};
  document.querySelector('#dlg-confirm').showModal();
  document.querySelector('#confirm-choices').replaceChildren();
  const i=document.createElement('input');i.type='checkbox';i.dataset.adapter='pi';i.checked=true;document.querySelector('#confirm-choices').appendChild(i);
  document.querySelector('#btn-confirm-ok').disabled=false;document.querySelector('#btn-confirm-ok').textContent='确认下发';
  return a.agentId;})()`);
await ev(sid,"document.querySelector('#btn-confirm-ok').click()");
await sleep(1200);
L("hung, dialog:", await ev(sid,"JSON.stringify({open:document.querySelector('#dlg-confirm').open,ok:document.querySelector('#btn-confirm-ok').textContent,cancel:document.querySelector('#btn-confirm-cancel').disabled})"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/C6-hung-dispatch.png");
// user presses the visible 取消 button (disabled) - real click via Input domain
const box = await ev(sid, `(()=>{const r=document.querySelector('#btn-confirm-cancel').getBoundingClientRect();return JSON.stringify({x:r.x+r.width/2,y:r.y+r.height/2})})()`);
const b = JSON.parse(box);
await cdp("Input.dispatchMouseEvent",{type:"mousePressed",x:b.x,y:b.y,button:"left",clickCount:1},sid);
await cdp("Input.dispatchMouseEvent",{type:"mouseReleased",x:b.x,y:b.y,button:"left",clickCount:1},sid);
await sleep(600);
L("after clicking 取消:", await ev(sid,"JSON.stringify({open:document.querySelector('#dlg-confirm').open,ok:document.querySelector('#btn-confirm-ok').textContent})"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/C6-cancel-click-noop.png");
// now press Escape (browser-native dialog cancel) -> does the in-flight dispatch get orphaned?
await cdp("Input.dispatchKeyEvent",{type:"keyDown",key:"Escape",code:"Escape",windowsVirtualKeyCode:27,nativeVirtualKeyCode:27},sid);
await cdp("Input.dispatchKeyEvent",{type:"keyUp",key:"Escape",code:"Escape",windowsVirtualKeyCode:27,nativeVirtualKeyCode:27},sid);
await sleep(900);
L("after Esc:", await ev(sid,"JSON.stringify({open:document.querySelector('#dlg-confirm').open,pending:app.pendingAction&&app.pendingAction.kind,step:app.scopeStep})"));
L("orphan check pendingAction:", await ev(sid,"JSON.stringify(app.pendingAction)"));
// release the hung request so the server actually performs the dispatch
await ev(sid,"(()=>{const r=window.__release;if(r){r();return 'released'}return 'no-op'})()");
await sleep(5000);
L("after backend completes:");
L("  pendingAction:", await ev(sid,"JSON.stringify(app.pendingAction)"));
L("  dlg open:", await ev(sid,"document.querySelector('#dlg-confirm').open"));
L("  dlg desc:", await ev(sid,"document.querySelector('#confirm-desc').textContent"));
L("  ok text:", await ev(sid,"document.querySelector('#btn-confirm-ok').textContent"), "disabled:", await ev(sid,"document.querySelector('#btn-confirm-ok').disabled"));
L("  toast:", await ev(sid,"document.querySelector('#toast').textContent"), "visible:", await ev(sid,"!document.querySelector('#toast').classList.contains('hidden')"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/C6-orphan-after-esc.png");
L("ERRS:", JSON.stringify(LOG.err.slice(0,5)), "window.__err:", await ev(sid,"JSON.stringify(window.__err)"));
process.exit(0);
