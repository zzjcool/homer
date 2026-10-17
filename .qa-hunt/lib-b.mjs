setTimeout(() => { console.error("TIMEOUT-GUARD-EXIT"); process.exit(9); }, 45_000).unref();
const V = await fetch("http://127.0.0.1:3000/json/version").then(r=>r.json());
const ws = new WebSocket(V.webSocketDebuggerUrl.replace("ws://0.0.0.0","ws://127.0.0.1"));
const pend = new Map(); let seq = 0;
await new Promise((res,rej)=>{ ws.onopen=res; ws.onerror=rej; });
ws.onmessage = m => { const d = JSON.parse(m.data); if (d.id && pend.has(d.id)) { pend.get(d.id)(d); pend.delete(d.id); } };
function raw(method, params={}, sid) {
  const id = ++seq;
  const p = new Promise(r => pend.set(id, r));
  const msg = { id, method, params };
  if (sid) msg.sessionId = sid;
  ws.send(JSON.stringify(msg));
  return Promise.race([p, new Promise((_,rj)=>setTimeout(()=>rj(new Error("CDP timeout "+method)),12000))]);
}
export async function attach() {
  const t = await raw("Target.getTargets");
  let page = t.result.targetInfos.find(x=>x.type==="page");
  if (!page) { const c = await raw("Target.createTarget",{url:"about:blank"}); page = {targetId: c.result.targetId}; await sleep(1000); }
  const sid = (await raw("Target.attachToTarget",{targetId:page.targetId,flatten:true})).result.sessionId;
  await raw("Page.enable",{},sid); await raw("Runtime.enable",{},sid);
  return sid;
}
export async function nav(sid, url) {
  await raw("Page.navigate",{url},sid);
  for (let i=0;i<60;i++){ await sleep(300); const r = await ev(sid,"document.readyState"); if (r==="complete") break; }
  await sleep(600);
}
export async function ev(sid, expr) {
  const r = await raw("Runtime.evaluate",{expression:expr, returnByValue:true, awaitPromise:true}, sid);
  if (r.result && r.result.exceptionDetails) return {__err: String(r.result.exceptionDetails.exception?.description || r.result.exceptionDetails.text).slice(0,300)};
  return r.result?.result?.value;
}
export async function shot(sid, path) {
  const r = await raw("Page.captureScreenshot",{format:"png"},sid);
  const { writeFileSync } = await import("node:fs");
  writeFileSync(path, Buffer.from(r.result.data, "base64"));
  return path;
}
export async function waitFor(sid, expr, ms=8000) {
  const t0 = Date.now();
  while (Date.now()-t0 < ms) { const v = await ev(sid, expr); if (v) return v; await sleep(250); }
  return false;
}
export const sleep = ms => new Promise(r=>setTimeout(r,ms));
export { raw as cdp };
