import { attach, nav, ev, sleep, shot, waitFor, LOG, cdp } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);
await cdp("Network.enable",{},sid);

// ---- C4: block /adapters endpoint -> openMachineView silent degrade
await cdp("Network.setBlockedURLs",{urls:["*adapters*"]},sid);
L("C4 blocked set");
await ev(sid, `(()=>{const c=[...document.querySelectorAll('.agent-card')].find(c=>![...c.querySelectorAll('button')].find(b=>b.textContent.trim()==='查看').disabled);if(!c)return 'none';[...c.querySelectorAll('button')].find(b=>b.textContent.trim()==='查看').click();return c.dataset.agentId})()`);
await waitFor(sid,"document.querySelector('#dlg-view').open && document.querySelector('#view-body').textContent.length>50",15000);
L("C4 view-body:", await ev(sid,"document.querySelector('#view-body').textContent.slice(0,700)"));
L("C4 toast:", await ev(sid,"document.querySelector('#toast').textContent"), "visible:", await ev(sid,"!document.querySelector('#toast').classList.contains('hidden')"));
L("C4 degrade line present:", await ev(sid,"document.querySelector('#view-body').textContent.includes('暂时无法读取这台机器的适配器停用状态')"));
L("C4 override buttons:", await ev(sid,"document.querySelectorAll('#view-body button[data-adapter-override]').length"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/11-view-adapters-blocked.png");
await ev(sid,"document.querySelector('#dlg-view').close()");
await cdp("Network.setBlockedURLs",{urls:[]},sid);

// ---- C5: refresh button feedback
L("C5 busy hidden class:", await ev(sid,"document.querySelector('#busy').className"), "| #busy text:", await ev(sid,"document.querySelector('#busy').textContent.slice(0,60)"));
L("C5 refreshed-at before:", await ev(sid,"document.querySelector('#refreshed-at').textContent"));
await ev(sid,"document.querySelector('#btn-refresh').click()");
await sleep(150);
L("C5 during refresh - busy visible:", await ev(sid,"!document.querySelector('#busy').classList.contains('hidden')"), "| btn-refresh disabled:", await ev(sid,"document.querySelector('#btn-refresh').disabled"), "| refreshed-at:", await ev(sid,"document.querySelector('#refreshed-at').textContent"));
await sleep(3000);
L("C5 after refresh - refreshed-at:", await ev(sid,"document.querySelector('#refreshed-at').textContent"));

// ---- C6: NaN / undefined leak via renderHostFacts with partial host
L("C6 normal hostSummary:", await ev(sid,"hostSummary(app.agents[0])"));
await ev(sid, `(()=>{const a=JSON.parse(JSON.stringify(app.agents[0]));a.host.memory={total:100,used:undefined};a.host.disks=[{used:5,total:10}];a.host.load={one:undefined,five:2};const s=hostSummary(a);const f=renderHostFacts(a);return JSON.stringify({summary:s,text:f.textContent.slice(0,300)})})()`);
L("C6 injected:", await ev(sid, `(()=>{const a=JSON.parse(JSON.stringify(app.agents[0]));a.host.memory={total:100,used:undefined};a.host.disks=[{used:5,total:10}];a.host.load={one:undefined,five:2};return JSON.stringify({summary:hostSummary(a),facts:renderHostFacts(a).textContent.slice(0,300)})})()`));
await ev(sid, `(()=>{const a=JSON.parse(JSON.stringify(app.agents[0]));a.host.memory={total:100,used:undefined};a.host.disks=[{used:5,total:10}];a.host.load={one:undefined,five:2};document.querySelector('#agent-list').replaceChildren();renderAgents([a],false);return 'rendered'})()`);
await sleep(300);
L("C6 rendered card text:", await ev(sid,"document.querySelector('.agent-card').textContent.slice(0,300)"));
L("C6 contains NaN:", await ev(sid,"document.querySelector('#agent-list').textContent.includes('NaN')"), "undefined:", await ev(sid,"document.querySelector('#agent-list').textContent.includes('undefined')"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/12-nan.png");
L("ERRS:", JSON.stringify(LOG.err.slice(0,6)));
process.exit(0);
