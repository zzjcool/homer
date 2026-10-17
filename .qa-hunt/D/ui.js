import http from 'node:http';
const v = await new Promise((res,rej)=>http.get('http://127.0.0.1:3000/json/version',r=>{let d='';r.on('data',c=>d+=c);r.on('end',()=>res(JSON.parse(d)))}).on('error',rej));
const ws = new WebSocket(v.webSocketDebuggerUrl.replace('ws://0.0.0.0','ws://127.0.0.1'));
let id=0; const pend=new Map();
function send(method,params={},sid){const m={id:++id,method,params};if(sid)m.sessionId=sid;ws.send(JSON.stringify(m));return new Promise((res,rej)=>{pend.set(m.id,{res,rej});setTimeout(()=>{if(pend.has(m.id)){pend.delete(m.id);rej(new Error('timeout '+method))}},60000)})}
ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id&&pend.has(m.id)){const p=pend.get(m.id);pend.delete(m.id);m.error?p.rej(new Error(JSON.stringify(m.error))):p.res(m.result)}};
await new Promise(r=>ws.onopen=r);
const targets = await send('Target.getTargets');
let page = targets.targetInfos.find(t=>t.type==='page');
if(!page){const c=await send('Target.createTarget',{url:'about:blank'});page={targetId:c.targetId};}
const {sessionId} = await send('Target.attachToTarget',{targetId:page.targetId,flatten:true});
await send('Page.enable',{},sessionId); await send('Runtime.enable',{},sessionId);
await send('Page.navigate',{url:'http://hub:7760/'},sessionId);
await new Promise(r=>setTimeout(r,3500));
// login if needed
let r1 = await send('Runtime.evaluate',{expression:`(function(){if(document.querySelector('#workspace')&&!document.querySelector('#workspace').classList.contains('hidden'))return 'in';document.querySelector('#in-login-pw').value='qa-admin-password';document.querySelector('#btn-login').click();return 'clicked';})()`,returnByValue:true},sessionId);
console.log('login:', JSON.stringify(r1.result?.value));
await new Promise(r=>setTimeout(r,3500));
const ev = async (expr) => { const r = await send('Runtime.evaluate',{expression:expr,returnByValue:true,awaitPromise:true},sessionId); return r.result?.value ?? r.exceptionDetails?.exception?.description ?? r; };
for (const e of process.argv.slice(2)) { console.log('--- ', e.slice(0,60)); console.log(JSON.stringify(await ev(e), null, 1)); }
ws.close();
