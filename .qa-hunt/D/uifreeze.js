import http from 'node:http';
const v = await new Promise((res,rej)=>http.get('http://127.0.0.1:3000/json/version',r=>{let d='';r.on('data',c=>d+=c);r.on('end',()=>res(JSON.parse(d)))}).on('error',rej));
const ws = new WebSocket(v.webSocketDebuggerUrl.replace('ws://0.0.0.0','ws://127.0.0.1'));
let id=0; const pend=new Map();
function send(m,p={},sid){const msg={id:++id,method:m,params:p};if(sid)msg.sessionId=sid;ws.send(JSON.stringify(msg));return new Promise((res,rej)=>{pend.set(msg.id,{res,rej});setTimeout(()=>{if(pend.has(msg.id)){pend.delete(msg.id);rej(new Error('timeout '+m))}},180000)})}
ws.onmessage=e=>{const mm=JSON.parse(e.data);if(mm.id&&pend.has(mm.id)){const p=pend.get(mm.id);pend.delete(mm.id);mm.error?p.rej(new Error(JSON.stringify(mm.error))):p.res(mm.result)}};
await new Promise(r=>ws.onopen=r);
const t = await send('Target.getTargets');
let page=t.targetInfos.find(x=>x.type==='page');
if(!page){const c=await send('Target.createTarget',{url:'about:blank'});page={targetId:c.targetId};}
const {sessionId}=await send('Target.attachToTarget',{targetId:page.targetId,flatten:true});
await send('Page.enable',{},sessionId); await send('Runtime.enable',{},sessionId);
await send('Page.navigate',{url:'http://hub:7760/'},sessionId);
await new Promise(r=>setTimeout(r,3500));
const ev=async e=>{const r=await send('Runtime.evaluate',{expression:e,returnByValue:true,awaitPromise:true},sessionId);return r.result?.value ?? r.exceptionDetails?.exception?.description ?? r;};
console.log('login:',await ev(`(function(){if(document.querySelector('#workspace')&&!document.querySelector('#workspace').classList.contains('hidden'))return 'in';document.querySelector('#in-login-pw').value='qa-admin-password';document.querySelector('#btn-login').click();return 'clicked';})()`));
await new Promise(r=>setTimeout(r,4000));
console.log('loggedIn:',await ev(`!document.querySelector('#workspace').classList.contains('hidden')`));
// Measure: does a blocked backend call freeze the whole UI thread? (fetch is async, so check busy overlay + whether refresh polls still update)
console.log('cards:',await ev(`document.querySelectorAll('.agent-card[data-agent-id]').length`));
// Simulate: start a long POST via fetch in page, meanwhile check if refresh polling still updates #refreshed-at
const res = await ev(`(async()=>{
  const t0=Date.now();
  const p = fetch('/api/agents/b6675d68ddf7-7920/pull',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'}).then(r=>r.status);
  // meanwhile poll whether UI thread is responsive
  const marks=[];
  for(let i=0;i<12;i++){ await new Promise(r=>setTimeout(r,1000)); marks.push([Date.now()-t0, document.querySelector('#refreshed-at')?.textContent||'', document.querySelector('#busy')?.classList.contains('hidden')]); }
  const st = await p;
  return {status:st, elapsed:Date.now()-t0, marks};
})()`);
console.log('RESULT:', JSON.stringify(res,null,1));
ws.close();
