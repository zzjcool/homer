const http = await import('node:http');
const v = await new Promise((res,rej)=>http.get('http://127.0.0.1:3000/json/version',r=>{let d='';r.on('data',c=>d+=c);r.on('end',()=>res(JSON.parse(d)))}).on('error',rej));
const ws = new globalThis.WebSocket(v.webSocketDebuggerUrl.replace('ws://0.0.0.0','ws://127.0.0.1'));
let id=0; const pend=new Map();
function send(method,params={},sid){const m={id:++id,method,params};if(sid)m.sessionId=sid;ws.send(JSON.stringify(m));
  return new Promise((res,rej)=>{pend.set(m.id,{res,rej});setTimeout(()=>{if(pend.has(m.id)){pend.delete(m.id);rej(new Error('timeout '+method))}},30000)})}
ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id&&pend.has(m.id)){const p=pend.get(m.id);pend.delete(m.id);m.error?p.rej(new Error(JSON.stringify(m.error))):p.res(m.result)}};
await new Promise(r=>ws.onopen=r);
const targets = await send('Target.getTargets');
let page = targets.targetInfos.find(t=>t.type==='page');
if(!page){const c=await send('Target.createTarget',{url:'about:blank'});page={targetId:c.targetId};}
const att = await send('Target.attachToTarget',{targetId:page.targetId,flatten:true});
const sid = att.sessionId;
await send('Page.enable',{},sid); await send('Runtime.enable',{},sid);
const ev = async (expr)=>{const r=await send('Runtime.evaluate',{expression:expr,returnByValue:true,awaitPromise:true},sid);
  if(r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description||JSON.stringify(r.exceptionDetails));
  return r.result?.value;};
const wait = ms=>new Promise(r=>setTimeout(r,ms));
await send('Page.navigate',{url:'http://hub:7760/'},sid); await wait(3500);
if (await ev(`document.querySelector('#workspace').classList.contains('hidden')`)) {
  await ev(`document.querySelector('#in-login-pw').value='qa-admin-password'; document.querySelector('#btn-login').click(); true`); await wait(3500);
}
await wait(3000);
const probe = `(() => {
  const cards=[...document.querySelectorAll('.agent-card[data-agent-id]')];
  const norm = c => ({
    agentId: c.dataset.agentId,
    hostname: c.querySelector('.hostname')?.textContent||null,
    cls: c.className,
    dot: c.querySelector('.dot')?.className||null,
    statusText: c.querySelector('.small.muted')?.textContent || c.querySelector('.counts')?.textContent || null,
    allSpans: [...c.querySelectorAll('span')].map(s=>s.textContent.trim()).filter(Boolean),
    buttons: [...c.querySelectorAll('button')].map(b=>b.textContent.trim()+(b.disabled?'(D)':'')),
  });
  return cards.map(norm);
})()`;
const cards = await ev(probe);
console.log(JSON.stringify(cards,null,1));
const revoked = cards.find(c=>c.agentId==='54d0c3373428-d82d');
const offline = cards.find(c=>c.agentId==='e363efdcf085-7b6d');
if (revoked && offline) {
  const key = c => JSON.stringify([c.cls,c.dot,c.statusText,c.buttons]);
  console.log('\n=== DIFF 判据 ===');
  console.log('revoked :', key(revoked));
  console.log('offline :', key(offline));
  console.log('渲染签名相同 =>', key(revoked)===key(offline));
}
ws.close();
