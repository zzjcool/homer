const http=require('http');
function get(p){return new Promise((res,rej)=>{http.get(p,r=>{let d='';r.on('data',c=>d+=c);r.on('end',()=>res(JSON.parse(d)))}).on('error',rej)})}
(async()=>{
const v=await get('http://127.0.0.1:3000/json/version');
const WebSocket=require('/root/code/pi-async/node_modules/ws');
const ws=new WebSocket(v.webSocketDebuggerUrl.replace('ws://0.0.0.0','ws://127.0.0.1'));
let id=0;const pend=new Map();
const send=(m,sid)=>new Promise((res,rej)=>{const i=++id;pend.set(i,{res,rej});ws.send(JSON.stringify({...m,id:i,sessionId:sid}))});
ws.on('message',m=>{const o=JSON.parse(m);if(o.id&&pend.has(o.id)){const{res,rej}=pend.get(o.id);pend.delete(o.id);o.error?rej(new Error(JSON.stringify(o.error))):res(o.result)}});
await new Promise(r=>ws.on('open',r));
const t=await send({method:'Target.getTargets'});
let page=t.targetInfos.find(x=>x.type==='page');
if(!page){const c=await send({method:'Target.createTarget',params:{url:'about:blank'}});page={targetId:c.targetId}}
const sid=(await send({method:'Target.attachToTarget',params:{targetId:page.targetId,flatten:true}})).sessionId;
const ev=e=>send({method:'Runtime.evaluate',params:{expression:e,returnByValue:true,awaitPromise:true}},sid).then(r=>r.result&&r.result.value);
await send({method:'Page.enable'},sid);
await send({method:'Page.navigate',params:{url:'http://hub:7760/'}},sid);
await new Promise(r=>setTimeout(r,3000));
await ev(`document.querySelector('#in-login-pw').value='qa-admin-password';document.querySelector('#btn-login').click();`);
await new Promise(r=>setTimeout(r,5000));
await ev(`document.querySelector('#btn-plugins').click()`);
await new Promise(r=>setTimeout(r,2000));
await ev(`window.__toasts=[];const o=window.toast;window.toast=function(m){window.__toasts.push(m);return o?o(m):null};window.confirm=()=>true;`);
// click dispatch-all for pi
console.log('click:',await ev(`(()=>{const r=[...document.querySelectorAll('#plugins-installed .plugin-row')].find(x=>x.dataset.pluginId==='pi');if(!r)return 'no-row';const b=[...r.querySelectorAll('button')].find(b=>/下发到全部机器/.test(b.textContent));if(!b)return 'no-btn';b.click();return 'clicked'})()`));
await new Promise(r=>setTimeout(r,15000));
console.log('TOASTS:',JSON.stringify(await ev(`window.__toasts`)));
console.log('PLUGIN-ERROR:',JSON.stringify(await ev(`(document.querySelector('#plugin-error')||{}).textContent`)));
console.log('plugin-error visible:',await ev(`(()=>{const e=document.querySelector('#plugin-error');return e? (!e.classList.contains('hidden') && e.textContent.trim().length>0) :'none'})()`));
const shot=await send({method:'Page.captureScreenshot',params:{format:'png'}},sid);
require('fs').writeFileSync('/root/code/homer-cli/.qa-hunt/artifacts-C/C2-ui-fanout-error.png',Buffer.from(shot.data,'base64'));
console.log('screenshot saved');
ws.close();
})().catch(e=>{console.error('ERR',e.message);process.exit(1)});
