// 自己的 CDP harness：登录 -> 采样机器卡片（D-C2/D-C4 判据）
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
console.log('logged-in?', await ev(`!!document.querySelector('#workspace') && !document.querySelector('#workspace').classList.contains('hidden')`));
if (!(await ev(`!document.querySelector('#workspace').classList.contains('hidden')`))) {
  await ev(`document.querySelector('#in-login-pw').value='qa-admin-password'; document.querySelector('#btn-login').click(); true`);
  await wait(3500);
}
console.log('after login hidden=', await ev(`document.querySelector('#workspace').classList.contains('hidden')`));
await wait(3000);

// ---- D-C2 判据：卡片可见文本（hostname + .aid + .ip + stale class + 按钮）
const probe = `(() => {
  const cards=[...document.querySelectorAll('.agent-card[data-agent-id]')];
  return cards.map(c=>{
    const aid=c.querySelector('.aid');
    return {
      agentId: c.dataset.agentId,
      aidVisible: aid ? aid.textContent : null,
      aidClipped: aid ? (aid.scrollWidth > aid.clientWidth + 1) : null,
      aidTruncatedText: aid ? (aid.scrollWidth > aid.clientWidth + 1) : null,
      hostname: c.querySelector('.hostname')?.textContent || null,
      ip: c.querySelector('.ip')?.textContent || null,
      stale: c.classList.contains('stale'),
      statusText: c.querySelector('.small.muted')?.textContent || c.querySelector('.counts')?.textContent || null,
      dotClass: c.querySelector('.dot')?.className || null,
      badge: [...c.querySelectorAll('span')].map(s=>s.textContent).filter(t=>/吊销|已吊销|移除|离线/.test(t)),
      buttons: [...c.querySelectorAll('button')].map(b=>({t:b.textContent.trim(), d:b.disabled})),
    };
  });
})()`;
console.log('--- D-C2/D-C4 卡片采样 ---');
console.log(JSON.stringify(await ev(probe), null, 1));
ws.close();
