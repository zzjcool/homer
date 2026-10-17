setTimeout(() => { console.error("TIMEOUT-GUARD"); process.exit(9); }, 25_000).unref();
const V = await fetch("http://127.0.0.1:3000/json/version").then(r=>r.json());
const ws = new WebSocket(V.webSocketDebuggerUrl.replace("ws://0.0.0.0","ws://127.0.0.1"));
const pend = new Map(); let seq = 0;
await new Promise((res,rej)=>{ ws.onopen=res; ws.onerror=rej; });
ws.onmessage = m => { const d = JSON.parse(m.data); if (d.id && pend.has(d.id)) { pend.get(d.id)(d); pend.delete(d.id); } };
function send(method, params={}, sid) {
  const id = ++seq;
  const p = new Promise(r => pend.set(id, r));
  ws.send(JSON.stringify({ id, method, params: params, sessionId: sid }));
  return Promise.race([p, new Promise((_,rj)=>setTimeout(()=>rj(new Error("timeout "+method)),15000))]);
}
export async function connect() {
  const t = await send("Target.getTargets");
  let page = t.result.targetInfos.find(x=>x.type==="page");
  if (!page) { const c = await send("Target.createTarget",{url:"about:blank"}); page = {targetId: c.result.targetId}; await new Promise(r=>setTimeout(r,800)); }
  const sid = (await send("Target.attachToTarget",{targetId:page.targetId,flatten:true})).result.sessionId;
  await send("Page.enable",{},sid);
  await send("Runtime.enable",{},sid);
  await send("Network.enable",{},sid);
  return { sid, send: (m,p)=>send(m,p,sid), ws };
}
export async function ev(sid, expr) {
  const r = await send("Runtime.evaluate",{expression:expr, returnByValue:true, awaitPromise:true}, sid);
  if (r.result && r.result.exceptionDetails) return {__err: JSON.stringify(r.result.exceptionDetails.exception && r.result.exceptionDetails.exception.description || r.result.exceptionDetails.text)};
  return r.result && r.result.result ? r.result.result.value : undefined;
}
export { send, ev as evalJs };
