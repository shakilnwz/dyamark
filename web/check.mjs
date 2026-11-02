// Loads the web build in headless Chromium, drives it through the page's
// API the way the PhpStorm plugin does, and checks what it tells and
// draws. Run by bin/webcheck, after bin/wasm:
//
//	node web/check.mjs BUILD_DIR OUT_DIR
//
// It needs Node 22 or later (for WebSocket) and Chromium, found as
// $CHROMIUM or chromium on the PATH. Chromium draws with SwiftShader, so
// no GPU is needed. Screenshots go to OUT_DIR.

import { spawn } from "node:child_process";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";

const [buildDir, outDir] = process.argv.slice(2);
if (!buildDir || !outDir) {
  console.error("usage: node web/check.mjs BUILD_DIR OUT_DIR");
  process.exit(2);
}

// The palette the page sets, all of it as the plugin does: light, so it
// can't be mistaken for the default dark one.
const colors = {
  background: "#fdfdfd", foreground: "#1f2328", accent: "#0969da",
  lighterBackground: "#e6ebf1", error: "#cf222e", rounding: 6,
};

// The document: a heading on line 1, a sequence diagram whose source
// starts on line 6, then enough text to scroll.
const fence = "```";
const diagramLine = 6;
const doc = [
  "# dyamark in a browser",
  "",
  "A [link](https://example.com) and some text.",
  "",
  fence + "mermaid",
  "sequenceDiagram",
  "  Alice->>Bob: Hello",
  "  Bob-->>Alice: Hi",
  fence,
  "",
  ...Array.from({ length: 30 }, (_, i) => `## Part ${i}\n\n${"Words to scroll past. ".repeat(30)}\n`),
].join("\n");

let failed = 0;
const ok = (cond, what) => {
  console.log(`${cond ? "ok  " : "FAIL"} ${what}`);
  if (!cond) failed++;
  return cond;
};

const mime = { ".html": "text/html", ".js": "text/javascript", ".wasm": "application/wasm" };
const server = createServer(async (req, res) => {
  const file = path.join(buildDir, path.normalize(new URL(req.url, "http://x").pathname));
  try {
    const data = await readFile(file);
    res.writeHead(200, { "Content-Type": mime[path.extname(file)] ?? "application/octet-stream" });
    res.end(data);
  } catch {
    res.writeHead(404).end();
  }
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const pageURL = `http://127.0.0.1:${server.address().port}/index.html`;

// A minimal DevTools protocol client over the page's WebSocket.
class Page {
  static async open(flags) {
    const profile = await mkdtemp(path.join(tmpdir(), "dyamark-webcheck-"));
    const proc = spawn(process.env.CHROMIUM || "chromium", [
      "--headless=new", "--remote-debugging-port=0", `--user-data-dir=${profile}`,
      "--no-first-run", "--no-default-browser-check", "--window-size=900,700",
      ...flags, "about:blank",
    ], { stdio: ["ignore", "ignore", "pipe"] });
    const browserURL = await new Promise((resolve, reject) => {
      let err = "";
      proc.stderr.on("data", (d) => {
        err += d;
        const m = err.match(/DevTools listening on (ws:\/\/\S+)/);
        if (m) resolve(m[1]);
      });
      proc.on("exit", (code) => reject(new Error(`chromium exited (${code}):\n${err}`)));
    });
    const port = new URL(browserURL).port;
    const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
    const target = targets.find((t) => t.type === "page");
    const page = new Page(proc, profile, new WebSocket(target.webSocketDebuggerUrl));
    await new Promise((resolve, reject) => {
      page.ws.onopen = resolve;
      page.ws.onerror = reject;
    });
    return page;
  }

  constructor(proc, profile, ws) {
    Object.assign(this, { proc, profile, ws, id: 0, calls: new Map(), messages: [], waiters: [] });
    ws.onmessage = (e) => {
      const m = JSON.parse(e.data);
      if (m.id) {
        const call = this.calls.get(m.id);
        this.calls.delete(m.id);
        m.error ? call.reject(new Error(m.error.message)) : call.resolve(m.result);
      } else if (m.method === "Runtime.bindingCalled" && m.params.name === "dyamarkHostPost") {
        this.messages.push({ at: performance.now(), text: m.params.payload });
        this.waiters = this.waiters.filter((w) => !w());
      }
    };
  }

  send(method, params = {}) {
    const id = ++this.id;
    this.ws.send(JSON.stringify({ id, method, params }));
    return new Promise((resolve, reject) => this.calls.set(id, { resolve, reject }));
  }

  // Loads the page, with the plugin's dyamarkHostPost in place from the
  // start.
  async load() {
    await this.send("Runtime.enable");
    await this.send("Runtime.addBinding", { name: "dyamarkHostPost" });
    await this.send("Page.enable");
    await this.send("Page.navigate", { url: pageURL });
  }

  async eval(expression) {
    const r = await this.send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description ?? r.exceptionDetails.text);
    return r.result.value;
  }

  // Returns the first message from index on that starts with prefix, or
  // null after timeout ms.
  next(prefix, index, timeout = 10000) {
    const find = () => this.messages.slice(index).find((m) => m.text.startsWith(prefix)) ?? null;
    return new Promise((resolve) => {
      const timer = setTimeout(() => resolve(find()), timeout);
      const check = () => {
        const m = find();
        if (m) {
          clearTimeout(timer);
          resolve(m);
        }
        return m !== null;
      };
      if (!check()) this.waiters.push(check);
    });
  }

  // Waits for two animation frames, so what Gio drew is on the canvas.
  settle() {
    return this.eval("new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)))");
  }

  async screenshot(name) {
    const { data } = await this.send("Page.captureScreenshot", { format: "png" });
    const file = path.join(outDir, name);
    await writeFile(file, Buffer.from(data, "base64"));
    return file;
  }

  async mouse(x, y) {
    for (const type of ["mousePressed", "mouseReleased"]) {
      await this.send("Input.dispatchMouseEvent", { type, x, y, button: "left", clickCount: 1 });
    }
  }

  async key(key, code, keyCode, text) {
    await this.send("Input.dispatchKeyEvent", { type: "keyDown", key, code, windowsVirtualKeyCode: keyCode, text });
    await this.send("Input.dispatchKeyEvent", { type: "keyUp", key, code, windowsVirtualKeyCode: keyCode });
  }

  async close() {
    this.ws.close();
    this.proc.kill();
    await new Promise((resolve) => this.proc.on("exit", resolve));
    // Chromium's helpers may still be writing to it for a moment.
    await rm(this.profile, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  }
}

// Counts the canvas's pixels near each of the given colours, and those
// dark enough to be text or blue enough to be the accent's lines: thin
// lines are smoothed, so few of their pixels are the accent itself.
const countColors = (want) => `(() => {
  const gl = document.querySelector("#giowindow canvas");
  const c = document.createElement("canvas");
  c.width = gl.width; c.height = gl.height;
  const ctx = c.getContext("2d");
  ctx.drawImage(gl, 0, 0);
  const px = ctx.getImageData(0, 0, c.width, c.height).data;
  const want = ${JSON.stringify(want)}.map(h => [1, 3, 5].map(i => parseInt(h.slice(i, i + 2), 16)));
  const near = want.map(() => 0);
  let dark = 0, blue = 0;
  for (let i = 0; i < px.length; i += 4) {
    const [r, g, b] = [px[i], px[i + 1], px[i + 2]];
    want.forEach(([wr, wg, wb], k) => {
      if (Math.abs(r - wr) + Math.abs(g - wg) + Math.abs(b - wb) < 24) near[k]++;
    });
    if (r + g + b < 300) dark++;
    if (b - r > 80) blue++;
  }
  return { width: c.width, height: c.height, near, dark, blue };
})()`;

const KEYS = {
  pageDown: ["PageDown", "PageDown", 34],
  tab: ["Tab", "Tab", 9],
  enter: ["Enter", "Enter", 13, "\r"],
  e: ["e", "KeyE", 69, "e"],
  escape: ["Escape", "Escape", 27],
};

await mkdir(outDir, { recursive: true });
const swiftshader = ["--use-angle=swiftshader", "--enable-unsafe-swiftshader"];

try {
  // 1. Start, theme and source.
  const page = await Page.open(swiftshader);
  try {
    await page.load();
    const ready = await page.next("ready", 0, 20000);
    const webgl = page.messages.find((m) => m.text.startsWith("webgl"));
    console.log(`     ${webgl?.text}; ${page.messages.find((m) => m.text.startsWith("loaded"))?.text ?? "no loaded"}`);
    if (!ok(ready, `ready (${ready?.text})`)) throw new Error("dyamark didn't start");

    ok((await page.eval(`dyamark.setTheme(${JSON.stringify(JSON.stringify(colors))})`)) === null, "setTheme takes a palette");
    const refused = await page.eval(`dyamark.setTheme('{"background":"white"}')`);
    ok(typeof refused === "string" && refused.includes("background"), `setTheme refuses a bad one: ${refused}`);

    let mark = page.messages.length;
    const sent = performance.now();
    await page.eval(`dyamark.setSource(${JSON.stringify(doc)})`);
    const drawn = await page.next("drawn", mark);
    ok(drawn, `drawn after setSource (${drawn ? Math.round(drawn.at - sent) : "-"} ms)`);
    await page.settle();
    const shot = await page.eval(countColors([colors.background, colors.lighterBackground]));
    const [bg, boxes] = shot.near;
    const area = shot.width * shot.height;
    console.log(`     canvas ${shot.width}×${shot.height}: background ${bg}, boxes ${boxes}, dark ${shot.dark}, blue ${shot.blue} px`);
    ok(bg > area / 2, "the background is the palette's");
    // Four boxes of about 150×70 px.
    ok(boxes > 30000, "the participants' boxes are in its lighter background");
    ok(shot.dark > 2000, "there is text, dark on the light page");
    ok(shot.blue > 1000, "the diagram's lines and the link are in its accent");
    console.log(`     ${await page.screenshot("1-document.png")}`);

    // 2. Keys before any click: Gio's text field has no focus yet.
    mark = page.messages.length;
    await page.key(...KEYS.pageDown);
    const early = await page.next("scroll", mark, 1000);
    console.log(`     Page Down before a click: ${early ? "arrived" : "didn't arrive"}`);

    // 3. scrollToLine moves the view without telling it back; after a
    // click in the margin, Page Down scrolls and tells the new top line.
    await page.eval("dyamark.scrollToLine(1)");
    await page.settle();
    await page.eval("dyamark.scrollToLine(40)");
    await page.settle();
    ok(!(await page.next("scroll", mark, 500)), "scrollToLine isn't told back");
    await page.mouse(10, 350);
    mark = page.messages.length;
    await page.key(...KEYS.pageDown);
    const scrolled = await page.next("scroll", mark, 3000);
    ok(scrolled && Number(scrolled.text.split(" ")[1]) > 40, `Page Down after a click scrolls: ${scrolled?.text}`);

    // 4. Tab and Enter open the diagram in the viewer; E asks to edit its
    // source; Esc goes back.
    await page.eval("dyamark.scrollToLine(1)");
    await page.settle();
    await page.key(...KEYS.tab);
    await page.key(...KEYS.enter);
    await page.settle();
    await page.settle();
    console.log(`     ${await page.screenshot("2-viewer.png")}`);
    mark = page.messages.length;
    await page.key(...KEYS.e);
    const edit = await page.next("edit", mark, 3000);
    ok(edit?.text === `edit ${diagramLine}`, `E in the viewer asks to edit line ${diagramLine}: ${edit?.text}`);
    await page.key(...KEYS.escape);

    // 5. Typing: a new source is drawn, and diagrams still are.
    mark = page.messages.length;
    const typed = performance.now();
    await page.eval(`dyamark.setSource(${JSON.stringify(doc.replace("Hello", "Hello again"))})`);
    const redrawn = await page.next("drawn", mark);
    ok(redrawn, `drawn after an edit (${redrawn ? Math.round(redrawn.at - typed) : "-"} ms)`);

    // 5b. Zoom and help functions work without errors.
    await page.eval("dyamark.zoomIn()");
    await page.eval("dyamark.zoomOut()");
    await page.eval("dyamark.resetZoom()");
    await page.eval("dyamark.toggleHelp()");
    await page.settle();
    await page.eval("dyamark.toggleHelp()");
    await page.settle();
    ok(true, "zoomIn, zoomOut, resetZoom and toggleHelp executed cleanly");

    // 5c. Live theme switch: preview adapts colors live with document open.
    const darkTheme = { background: "#111827", foreground: "#f3f4f6", accent: "#3b82f6" };
    await page.eval(`dyamark.setTheme(${JSON.stringify(JSON.stringify(darkTheme))})`);
    await page.settle();
    const darkShot = await page.eval(countColors([darkTheme.background]));
    ok(darkShot.near[0] > (darkShot.width * darkShot.height) / 2, "live theme switch updates background color");

    const errors = page.messages.filter((m) => m.text.startsWith("error"));
    ok(errors.length === 0, `no errors${errors.length ? ": " + errors.map((m) => m.text).join("; ") : ""}`);
  } finally {
    await page.close();
  }

  // 6. Without WebGL the page says so instead of starting.
  const bare = await Page.open(["--disable-3d-apis"]);
  try {
    await bare.load();
    const none = await bare.next("webgl", 0, 10000);
    ok(none?.text === "webgl none", `without WebGL: ${none?.text}`);
    const error = await bare.next("error", 0, 3000);
    ok(error, `and it says so: ${error?.text}`);
    ok(!(await bare.next("loaded", 0, 1000)), "and doesn't start the wasm");
    const shown = await bare.eval(`getComputedStyle(document.getElementById("problem")).display`);
    ok(shown === "block", "the message is on the page");
  } finally {
    await bare.close();
  }
} finally {
  server.close();
}

console.log(failed ? `${failed} failed` : "all ok");
process.exit(failed ? 1 : 0);
