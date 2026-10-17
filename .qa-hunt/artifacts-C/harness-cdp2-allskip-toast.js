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
console.log('login-needed?',await ev(`document.querySelector('#workspace').classList.contains('hidden')`));
await ev(`document.querySelector('#in-login-pw').value='qa-admin-password';document.querySelector('#btn-login').click();`);
await new Promise(r=>setTimeout(r,5000));
console.log('ws-hidden-after-login?',await ev(`document.querySelector('#workspace').classList.contains('hidden')`));
// open plugin manager
await ev(`document.querySelector('#btn-plugins').click()`);
await new Promise(r=>setTimeout(r,2000));
console.log('dlg-plugins open?',await ev(`document.querySelector('#dlg-plugins').open`));
console.log('installed rows:',await ev(`Array.from(document.querySelectorAll('#plugins-installed .plugin-row')).map(r=>r.getAttribute('data-plugin-id')||r.textContent.slice(0,30)).join(' | ')`));
// find "下发到全部机器" button for pi
const btnInfo=await ev(`(()=>{const rows=[...document.querySelectorAll('#plugins-installed *')];const bs=rows.filter(e=>e.tagName==='BUTTON'&&/下发到全部机器/.test(e.textContent));return bs.map(b=>b.id||b.className+':'+b.textContent.trim()+':near='+(b.closest('[data-plugin-id]')?b.closest('[data-plugin-id]').getAttribute('data-plugin-id'):'?'));})()`);
console.log('dispatch-buttons:',JSON.stringify(btnInfo));
// disable all adapters on all online agents first
console.log('disable-all:',await ev(`(async()=>{const ag=await(await fetch('/api/agents')).json();const o=[];for(const a of ag.agents){if(a.stale)continue;for(const ad of ['pi','herdr','opencode','vscode']){const r=await fetch('/api/agents/'+a.agentId+'/adapters/'+ad,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({disabled:true})});o.push(a.agentId+'/'+ad+':'+r.status)}}return o.join(' ')})()`));
// click the dispatch-all button, capture toast
await ev(`window.__toasts=[];const orig=window.toast;window.toast=function(m){window.__toasts.push(m);return orig?orig(m):null};`);
console.log('click:',await ev(`(()=>{const bs=[...document.querySelectorAll('#plugins-installed button')].filter(b=>/下发到全部机器/.test(b.textContent));if(!bs.length)return 'no-button';window.confirm=()=>true;bs[0].click();return 'clicked:'+bs[0].textContent.trim()})()`));
await new Promise(r=>setTimeout(r,12000));
console.log('TOASTS:',JSON.stringify(await ev(`window.__toasts`)));
console.log('plugin-error:',await ev(`(document.querySelector('#plugin-error')||{}).textContent`));
ws.close();
})().catch(e=>{console.error('ERR',e.message);process.exit(1)});
