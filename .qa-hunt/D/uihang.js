import http from 'node:http';
const v = await new Promise((res,rej)=>http.get('http://127.0.0.1:3000/json/version',r=>{let d='';r.on('data',c=>d+=c);r.on('end',()=>res(JSON.parse(d)))}).on('error',rej));
const ws = new WebSocket(v.webSocketDebuggerUrl.replace('ws://0.0.0.0','ws://127.0.0.1'));
let id=0; const pend=new Map();
function send(m,p={},sid){const msg={id:++id,method:m,params:p};if(sid)msg.sessionId=sid;ws.send(JSON.stringify(msg));return new Promise((res,rej)=>{pend.set(msg.id,{res,rej});setTimeout(()=>{if(pend.has(msg.id)){pend.delete(msg.id);rej(new Error('TIMEOUT '+m))}},25000)})}
ws.onmessage=e=>{const mm=JSON.parse(e.data);if(mm.id&&pend.has(mm.id)){const p=pend.get(mm.id);pend.delete(mm.id);mm.error?p.rej(new Error(JSON.stringify(mm.error))):p.res(mm.result)}};
await new Promise(r=>ws.onopen=r);
const t=await send('Target.getTargets'); let page=t.targetInfos.find(x=>x.type==='page');
if(!page){const c=await send('Target.createTarget',{url:'about:blank'});page={targetId:c.targetId};}
const {sessionId}=await send('Target.attachToTarget',{targetId:page.targetId,flatten:true});
await send('Page.enable',{},sessionId); await send('Runtime.enable',{},sessionId);
await send('Page.navigate',{url:'http://hub:7760/'},sessionId);
await new Promise(r=>setTimeout(r,3500));
const ev=async e=>{try{const r=await send('Runtime.evaluate',{expression:e,returnByValue:true,awaitPromise:true},sessionId);return r.result?.value ?? ('EXC:'+(r.exceptionDetails?.exception?.description||''))}catch(err){return 'ERR:'+err.message}};
await ev(`(function(){if(document.querySelector('#workspace')&&!document.querySelector('#workspace').classList.contains('hidden'))return 'in';document.querySelector('#in-login-pw').value='qa-admin-password';document.querySelector('#btn-login').click();return 'clicked';})()`);
await new Promise(r=>setTimeout(r,4000));
console.log('loggedIn:',await ev(`!document.querySelector('#workspace').classList.contains('hidden')`));
console.log('cards:',await ev(`[...document.querySelectorAll('.agent-card[data-agent-id]')].map(c=>c.dataset.agentId+':'+(c.classList.contains('stale')?'stale':'live'))`));
// open dispatch dialog for a LIVE machine
console.log('openDispatch:', await ev(`(function(){const a=(window.app&&window.app.agents)||[]; const live=a.filter(x=>!x.stale); if(!live.length) return 'no-live'; const t=live[0]; window.__t=t.agentId; return t.agentId;})()`));
console.log('clickDispatch:', await ev(`(function(){const card=[...document.querySelectorAll('.agent-card[data-agent-id]')].find(c=>c.dataset.agentId===window.__t); if(!card)return 'nocard'; const btns=[...card.querySelectorAll('button')]; const b=btns.find(x=>x.textContent.trim()==='下发')||btns.find(x=>x.textContent.trim()==='收取'); if(!b)return 'nobtn:'+btns.map(x=>x.textContent.trim()); b.click(); return 'clicked:'+b.textContent.trim();})()`));
await new Promise(r=>setTimeout(r,3000));
console.log('dialogOpen:', await ev(`(function(){const d=document.querySelector('#dlg-confirm');return {open:d&&d.open, desc:document.querySelector('#confirm-desc')?.textContent, ok:document.querySelector('#btn-confirm-ok')?.textContent, okDisabled:document.querySelector('#btn-confirm-ok')?.disabled};})()`));
// click OK to actually dispatch
console.log('clickOK:', await ev(`(function(){const ok=document.querySelector('#btn-confirm-ok'); if(!ok)return 'nook'; if(ok.disabled) return 'ok-disabled'; ok.click(); return 'clicked';})()`));
const marks=[];
for(let i=0;i<10;i++){
  await new Promise(r=>setTimeout(r,4000));
  marks.push(await ev(`(function(){return {t:new Date().toISOString(), ok:document.querySelector('#btn-confirm-ok')?.textContent, cancelDisabled:document.querySelector('#btn-confirm-cancel')?.disabled, err:document.querySelector('#confirm-err')?.textContent, busyHidden:document.querySelector('#busy')?.classList.contains('hidden'), refreshed:document.querySelector('#refreshed-at')?.textContent};})()`));
}
console.log('MARKS:', JSON.stringify(marks,null,1));
ws.close();
