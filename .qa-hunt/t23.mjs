import { attach, nav, ev, sleep, shot, LOG, cdp } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
for(let i=0;i<40;i++){await sleep(300); if(await ev(sid,"!document.querySelector('#workspace').classList.contains('hidden')"))break;}
const L=(...a)=>console.log(...a);
await cdp("Emulation.setDeviceMetricsOverride",{width:1280,height:900,deviceScaleFactor:1,mobile:false},sid);
// ---- M1: open 更多 menu, then let poll replace the node mid-interaction -> click lost
await ev(sid,"window.scrollTo(0,0);document.querySelector('#sec-agents').scrollIntoView();refresh()"); await sleep(3500);
await ev(sid,`(()=>{const d=document.querySelector('.agent-card details.agent-more');d.open=true;return 'opened'})()`);
await sleep(500);
const btn = await ev(sid, `(()=>{const b=[...document.querySelectorAll('.agent-card details.agent-more[open] button')].find(b=>b.textContent.trim()==='吊销');if(!b)return 'none';
  const r=b.getBoundingClientRect();return JSON.stringify({x:r.x+r.width/2,y:r.y+r.height/2,text:b.textContent.trim()})})()`);
L("M1 target btn:", btn);
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/M1-menu-open-before-poll.png");
// fire refresh (what the 30s poll does), wait, THEN click at the old coordinates
await ev(sid,"refresh()");
await sleep(2500);
const {x,y} = JSON.parse(btn);
await cdp("Input.dispatchMouseEvent",{type:"mousePressed",x,y,button:"left",clickCount:1},sid);
await cdp("Input.dispatchMouseEvent",{type:"mouseReleased",x,y,button:"left",clickCount:1},sid);
await sleep(900);
L("M1 dialogs after stale click:", await ev(sid,"JSON.stringify(window.__dlg)"));
L("M1 confirm open:", await ev(sid,"document.querySelector('#dlg-confirm').open"));
L("M1 menu open now:", await ev(sid,"document.querySelector('.agent-card details.agent-more').open"));
L("M1 elementFromPoint at old coords:", await ev(sid,`(function(){const e=document.elementFromPoint(${x},${y});return e?e.tagName+'.'+(e.className||'')+'|'+(e.textContent||'').slice(0,20):'null'})()`));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/M1-after-stale-click.png");

// ---- M2: double-click dispatch OK -> single backend call?
await ev(sid, `(()=>{const of=window.fetch.bind(window);window.__pullCalls=0;
  window.fetch=(u,o)=>{if(String(u).includes('/pull')){window.__pullCalls++;return new Promise(r=>setTimeout(()=>of(u,o).then(r),1500))}return of(u,o)};return 'p'})()`);
await ev(sid, `(()=>{const a=app.agents.find(a=>!a.stale);app.scopeDirection='dispatch';app.scopeStep='choose';app.scopeAgent=a;
  app.scopeChoices=[{id:'pi',enabled:true}];app.scopeKeys=[];app.pendingAction={kind:'dispatch-one',agentId:a.agentId,name:'X'};
  document.querySelector('#dlg-confirm').showModal();
  document.querySelector('#confirm-choices').replaceChildren();
  const i=document.createElement('input');i.type='checkbox';i.dataset.adapter='pi';i.checked=true;document.querySelector('#confirm-choices').appendChild(i);
  document.querySelector('#btn-confirm-ok').disabled=false;document.querySelector('#btn-confirm-ok').textContent='确认下发';return 'ready'})()`);
const ob = JSON.parse(await ev(sid,`(()=>{const r=document.querySelector('#btn-confirm-ok').getBoundingClientRect();return JSON.stringify({x:r.x+r.width/2,y:r.y+r.height/2})})()`));
for(let i=0;i<2;i++){ await cdp("Input.dispatchMouseEvent",{type:"mousePressed",x:ob.x,y:ob.y,button:"left",clickCount:i+1},sid);
  await cdp("Input.dispatchMouseEvent",{type:"mouseReleased",x:ob.x,y:ob.y,button:"left",clickCount:i+1},sid); await sleep(40); }
await sleep(3500);
L("M2 pull calls after double click:", await ev(sid,"window.__pullCalls"), "| ok:", await ev(sid,"document.querySelector('#btn-confirm-ok').textContent"));
L("ERRS:", JSON.stringify(LOG.err.slice(0,5)));
process.exit(0);
