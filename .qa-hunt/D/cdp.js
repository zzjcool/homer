// CDP helper: node .qa-hunt/D/cdp.js '<js expr>' [navUrl]
const http = await import('node:http');
const v = await new Promise((res,rej)=>http.get('http://127.0.0.1:3000/json/version',r=>{let d='';r.on('data',c=>d+=c);r.on('end',()=>res(JSON.parse(d)))}).on('error',rej));
const WS = globalThis.WebSocket;
const ws = new WS(v.webSocketDebuggerUrl.replace('ws://0.0.0.0','ws://127.0.0.1'));
let id=0; const pend=new Map(); const ev=[];
let sessionId=null;
function send(method,params={},sid){const m={id:++id,method,params};if(sid)m.sessionId=sid;ws.send(JSON.stringify(m));return new Promise((res,rej)=>{pend.set(m.id,{res,rej});setTimeout(()=>{if(pend.has(m.id)){pend.delete(m.id);rej(new Error('timeout '+method))}},30000)})}
ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id&&pend.has(m.id)){const p=pend.get(m.id);pend.delete(m.id);m.error?p.rej(new Error(JSON.stringify(m.error))):p.res(m.result)}else{ev.push(m)}};
await new Promise(r=>ws.onopen=r);
const targets = await send('Target.getTargets');
let page = targets.targetInfos.find(t=>t.type==='page');
if(!page){const c=await send('Target.createTarget',{url:'about:blank'});page={targetId:c.targetId};}
const att = await send('Target.attachToTarget',{targetId:page.targetId,flatten:true});
sessionId = att.sessionId;
await send('Page.enable',{},sessionId);
await send('Runtime.enable',{},sessionId);
const expr = process.argv[2];
if(process.argv[3]) { await send('Page.navigate',{url:process.argv[3]},sessionId); await new Promise(r=>setTimeout(r,3000)); }
const r = await send('Runtime.evaluate',{expression:expr,returnByValue:true,awaitPromise:true},sessionId);
console.log(JSON.stringify(r.result?.value ?? r.exceptionDetails?.exception?.description ?? r, null, 1));
ws.close();
