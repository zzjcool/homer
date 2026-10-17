import { attach, nav, ev, sleep, shot, waitFor, LOG } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);

// open 处理冲突 dialog on a machine with conflicts
const openResolve = async () => {
  await ev(sid, `(()=>{const c=[...document.querySelectorAll('.agent-card')].find(c=>[...c.querySelectorAll('button')].some(b=>b.textContent.trim()==='处理冲突'));if(!c)return 'none';[...c.querySelectorAll('button')].find(b=>b.textContent.trim()==='处理冲突').click();return c.dataset.agentId})()`);
  await waitFor(sid,"document.querySelector('#confirm-choices').children.length>0",12000);
};
L("R target:", await openResolve());
L("R title:", await ev(sid,"document.querySelector('#confirm-title').textContent"));
L("R desc:", await ev(sid,"document.querySelector('#confirm-desc').textContent"));
L("R adapters:", await ev(sid,"JSON.stringify([...document.querySelectorAll('#confirm-choices input[data-adapter]')].map(i=>[i.dataset.adapter,i.checked,i.disabled]))"));
L("R radios:", await ev(sid,"JSON.stringify([...document.querySelectorAll('#confirm-choices input[data-resolution-adapter]')].map(i=>[i.dataset.resolutionAdapter,i.value,i.checked,i.disabled]))"));
L("R ok/alt:", await ev(sid,"JSON.stringify({ok:[document.querySelector('#btn-confirm-ok').textContent,document.querySelector('#btn-confirm-ok').disabled],alt:[document.querySelector('#btn-confirm-alt').textContent,document.querySelector('#btn-confirm-alt').disabled,document.querySelector('#btn-confirm-alt').classList.contains('hidden')]})"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/08-resolve.png");

// R2: uncheck all → OK silent no-op? (updateScopeConfirm disables; but test clicking alt)
await ev(sid,"[...document.querySelectorAll('#confirm-choices input[data-adapter]')].forEach(i=>{i.checked=false;i.dispatchEvent(new Event('change',{bubbles:true}))})");
await sleep(400);
L("R2 after uncheck: ok.disabled=", await ev(sid,"document.querySelector('#btn-confirm-ok').disabled"), "alt.disabled=", await ev(sid,"document.querySelector('#btn-confirm-alt').disabled"));

// R3: check a conflicting adapter but leave radio unselected → click OK
await ev(sid, `(()=>{const boxes=[...document.querySelectorAll('#confirm-choices input[data-adapter]')];const b=boxes.find(b=>!b.disabled);b.checked=true;b.dispatchEvent(new Event('change',{bubbles:true}));return b.dataset.adapter})()`);
await sleep(400);
L("R3 picked:", await ev(sid,"JSON.stringify(selectedPicks())"));
L("R3 ok.disabled:", await ev(sid,"document.querySelector('#btn-confirm-ok').disabled"), "err:", await ev(sid,"document.querySelector('#confirm-err').textContent"));

// R4: select radio center → OK enabled, click OK (记录决定)
await ev(sid, `(()=>{const r=[...document.querySelectorAll('#confirm-choices input[data-resolution-adapter]')].find(r=>!r.disabled);if(!r)return 'none';r.checked=true;r.dispatchEvent(new Event('change',{bubbles:true}));return r.value})()`);
await sleep(400);
L("R4 picked:", await ev(sid,"JSON.stringify(selectedPicks())"), "ok.disabled:", await ev(sid,"document.querySelector('#btn-confirm-ok').disabled"));
await ev(sid,"window.__dlg.length=0");
await ev(sid,"document.querySelector('#btn-confirm-ok').click()");
await sleep(2500);
L("R4 dlg open after OK:", await ev(sid,"document.querySelector('#dlg-confirm').open"));
L("R4 toast:", await ev(sid,"document.querySelector('#toast').textContent"));
L("R4 desc:", await ev(sid,"document.querySelector('#confirm-desc').textContent"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/09-resolve-after-ok.png");
L("ERRS:", JSON.stringify(LOG.err.slice(0,6)), JSON.stringify(await ev(sid,"JSON.stringify(window.__err)")));
process.exit(0);
