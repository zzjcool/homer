setTimeout(() => process.exit(9), 30_000).unref();
import { connect, sleep, login } from "./repro2-lib.mjs";
const B = process.argv[2] || "http://hub:7760";
const { send, ev, sid, shot } = await connect();
await send("Runtime.enable", {}, sid);
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Page.navigate", { url: B + "/" }, sid);
for (let i = 0; i < 40; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
console.log("url:", await ev("location.href"), "| title:", await ev("document.title"));
console.log("secure:", await ev("window.isSecureContext"), "| clipboard typeof:", await ev("typeof navigator.clipboard"));
console.log("workspaceHidden:", await ev("document.querySelector('#workspace').classList.contains('hidden')"));
const ok = await login(ev);
console.log("loggedIn:", ok, "| cards:", await ev("document.querySelectorAll('.agent-card').length"));
await shot("boot.png");
process.exit(0);
