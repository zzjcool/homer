// repro-2 独立 harness：CDP 直连，不复用 hunter 的 lib2.mjs
export async function connect() {
  const v = await fetch("http://127.0.0.1:3000/json/version").then(r => r.json());
  const url = v.webSocketDebuggerUrl.replace("ws://0.0.0.0", "ws://127.0.0.1");
  const ws = new WebSocket(url);
  const pend = new Map(); let seq = 0;
  const events = [];
  const send = (method, params = {}, sid) => new Promise((res, rej) => {
    const id = ++seq;
    pend.set(id, { res, rej });
    ws.send(JSON.stringify({ id, method, params, sessionId: sid }));
    setTimeout(() => { if (pend.has(id)) { pend.delete(id); rej(new Error("timeout " + method)); } }, 20000);
  });
  await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });
  ws.onmessage = m => {
    const msg = JSON.parse(m.data);
    if (msg.id && pend.has(msg.id)) { const p = pend.get(msg.id); pend.delete(msg.id); msg.error ? p.rej(new Error(JSON.stringify(msg.error))) : p.res(msg.result); return; }
    if (msg.method) events.push(msg);
  };
  const c = await send("Target.createTarget", { url: "about:blank" });
  const page = { targetId: c.targetId };
  const sid = (await send("Target.attachToTarget", { targetId: page.targetId, flatten: true })).sessionId;
  const ev = (expr, sid2 = sid) => send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }, sid2).then(r => {
    if (r && r.exceptionDetails) throw new Error("EVAL-EXC " + ((r.exceptionDetails.exception && r.exceptionDetails.exception.description) || r.exceptionDetails.text));
    return r && r.result ? r.result.value : undefined;
  });
  const waitFor = async (expr, limitMs = 15000, step = 250) => {
    const t0 = Date.now();
    while (Date.now() - t0 < limitMs) { try { if (await ev(expr)) return true; } catch (_) {} await new Promise(r => setTimeout(r, step)); }
    throw new Error("waitFor timeout: " + expr);
  };
  const shot = async (name) => {
    const dir = process.env.SHOTDIR || "/root/code/homer-cli/.qa-hunt/shots-repro2";
    const r = await send("Page.captureScreenshot", { format: "png" }, sid);
    const fs = await import("node:fs");
    fs.writeFileSync(dir + "/" + name, Buffer.from(r.data, "base64"));
    return dir + "/" + name;
  };
  return { send, ev, waitFor, shot, events, sid, targetId: page.targetId, ws };
}
export const sleep = ms => new Promise(r => setTimeout(r, ms));
export async function login(ev, pw) {
  for (let i = 0; i < 40; i++) {
    await sleep(300);
    try { if (await ev("!document.querySelector(\"#workspace\").classList.contains(\"hidden\")")) return true; } catch (_) {}
  }
  await ev("document.querySelector('#in-login-pw').value=" + JSON.stringify(pw || "qa-admin-password"));
  await ev("document.querySelector('#btn-login').click()");
  for (let i = 0; i < 40; i++) {
    await sleep(300);
    try { if (await ev("!document.querySelector(\"#workspace\").classList.contains(\"hidden\")")) return true; } catch (_) {}
  }
  return false;
}
