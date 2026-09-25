const $ = (id) => document.getElementById(id);

const urlEl = $("url");
const pasteUrlBtn = $("pasteUrl");
const destEl = $("dest");
const browseBtn = $("browse");
const bitrateEl = $("bitrate");
const bitrateRow = $("bitrateRow");
const resolutionEl = $("resolution");
const resolutionRow = $("resolutionRow");
const loginBtn = $("login");
const loginDoneBtn = $("loginDone");
const qualityHint = $("qualityHint");
const coverEl = $("cover");
const downloadBtn = $("download");
const cancelBtn = $("cancel");
const updateBtn = $("update");
const quitBtn = $("quit");
const openFolderBtn = $("openFolder");
const dlState = $("dlState");
const dlPath = $("dlPath");
const dlTrack = $("dlTrack");
const logEl = $("log");
const toolsEl = $("tools");

const DEST_KEY = "ytd_dest_root";
let isBusy = false;
let quitting = false;
let lastFolder = "";
let probeTimer = 0;
let probeSeq = 0;
let probeCache = null;
let heldEta = "";
let heldPct = -1;

function selectedFormat() {
  return document.querySelector('input[name="format"]:checked').value;
}

const dlFill = $("dlFill");

function setDlUI({ state, path, mode, enableOpen, percent }) {
  if (state != null) dlState.textContent = state;
  if (path != null) {
    dlPath.textContent = path || "—";
    dlPath.title = path || "";
  }
  if (mode !== undefined) {
    dlTrack.classList.remove("active", "done", "fail", "meter");
    if (mode) dlTrack.classList.add(mode);
    if (percent == null) dlFill.style.width = "";
  }
  if (percent != null) {
    const n = Math.max(0, Math.min(100, percent));
    dlTrack.classList.add("meter");
    dlFill.style.width = n + "%";
  }
  if (enableOpen != null) {
    openFolderBtn.disabled = !enableOpen;
  }
}

function applyDownloadLine(msg) {
  const dest = msg.match(/^\[download\] Destination: (.+)$/);
  if (dest) {
    setDlUI({ path: dest[1].trim(), state: "Fájl írása…", mode: "active" });
    return;
  }
  if (msg.startsWith("[download] Sleeping")) {
    setDlUI({ state: "Várakozás a szerverre…", mode: "active" });
    return;
  }
  if (msg.startsWith("[FixupM3u8]")) {
    setDlUI({ state: "Összefűzés…", percent: 100, mode: "active" });
    return;
  }
  const prog = msg.match(
    /^\[download\]\s+([\d.]+)%\s+of\s+\S[\s\S]*?\bat\s+(\S+)(?:\s+ETA\s+(\S+))?(?:\s+\(frag\s+(\d+\/\d+)\))?/
  );
  if (!prog) return null;
  const pct = parseFloat(prog[1]);
  if (!Number.isFinite(pct)) return null;
  const bits = [pct.toFixed(pct >= 10 ? 0 : 1).replace(/\.0$/, "") + "%", prog[2]];
  if (prog[3] && prog[3] !== "Unknown") bits.push("ETA " + prog[3]);
  if (prog[4]) bits.push(prog[4]);
  setDlUI({ state: bits.join(" · "), percent: pct, mode: "active" });
  return pct;
}

function progressBar(pct) {
  const width = 28;
  const filled = Math.round(Math.max(0, Math.min(100, pct)) / 100 * width);
  return "[" + "#".repeat(filled) + "-".repeat(width - filled) + "]";
}

function setBusy(busy) {
  isBusy = !!busy;
  downloadBtn.disabled = busy;
  updateBtn.disabled = busy;
  browseBtn.disabled = busy;
  pasteUrlBtn.disabled = busy;
  cancelBtn.disabled = !busy;
  syncAccountButtons();
  if (busy) {
    heldEta = "";
    heldPct = -1;
    openFolderBtn.disabled = true;
    setDlUI({ state: "Folyamatban…", mode: "active", enableOpen: false });
  }
}

const logLines = [];

function renderLog() {
  logEl.textContent = logLines.length ? logLines.join("\n") + "\n" : "";
  logEl.scrollTop = logEl.scrollHeight;
}

function appendLog(line) {
  logLines.push(line);
  renderLog();
}

function realEta(eta) {
  return !!eta && eta !== "Unknown" && eta !== "NA" && eta !== "00:00" && eta !== "00:00:00";
}

function trackEta(pct, eta) {
  if (Number.isFinite(pct) && heldPct >= 0 && pct + 8 < heldPct) {
    heldEta = "";
  }
  if (Number.isFinite(pct)) heldPct = pct;
  if (realEta(eta)) heldEta = eta;
  return heldEta;
}

function isProgressBar(line) {
  return /^\[[#\-]+\](?: ETA \S+)?$/.test(line);
}

function setProgressLog(bar) {
  const last = logLines[logLines.length - 1];
  if (last && isProgressBar(last)) {
    logLines[logLines.length - 1] = bar;
  } else {
    logLines.push(bar);
  }
  renderLog();
}

function syncFormatOptions() {
  const format = selectedFormat();
  const mp3 = format === "mp3";
  const mp4 = format === "mp4";
  bitrateRow.classList.toggle("disabled", !mp3);
  bitrateEl.disabled = !mp3;
  resolutionRow.classList.toggle("disabled", !mp4);
  resolutionEl.disabled = !mp4;
  syncAccountButtons();
  applyProbeCache();
  scheduleProbe(true);
}

function maxListedHeight(heights) {
  let best = 0;
  for (const h of heights) if (h > best) best = h;
  return best;
}

function qualityMessage(heights) {
  const avail = maxListedHeight(heights);
  if (!avail) return "A felbontást nem sikerült ellenőrizni.";
  return `Elérhető: ${avail}p.`;
}

function setQualityHint(text) {
  qualityHint.hidden = !text;
  qualityHint.textContent = text || "";
}

let coverToken = 0;

function setCover(url) {
  const token = ++coverToken;
  if (!url) {
    coverEl.hidden = true;
    coverEl.removeAttribute("src");
    return;
  }
  coverEl.hidden = false;
  coverEl.onerror = () => {
    if (token !== coverToken) return;
    coverEl.hidden = true;
  };
  const proxied = "/api/thumb?u=" + encodeURIComponent(url);
  if (coverEl.getAttribute("src") !== proxied) coverEl.src = proxied;
}

function applyProbeCache() {
  if (!probeCache || probeCache.url !== urlEl.value.trim()) {
    setCover("");
    return false;
  }
  setCover(probeCache.thumbnail || "");
  if (selectedFormat() !== "mp4") {
    setQualityHint("");
    return true;
  }
  if (probeCache.playlist || probeCache.needLogin || !probeCache.heights) {
    setQualityHint(probeCache.message || "");
    return true;
  }
  setQualityHint(qualityMessage(probeCache.heights));
  return true;
}

function scheduleProbe(immediate) {
  clearTimeout(probeTimer);
  const url = urlEl.value.trim();
  if (!url) {
    probeCache = null;
    setQualityHint("");
    setCover("");
    return;
  }
  if (applyProbeCache() && (probeCache.heights || probeCache.playlist || probeCache.needLogin)) {
    return;
  }
  const ready = /youtu\.be\/|youtube\.com\//i.test(url);
  probeTimer = setTimeout(() => runProbe(), immediate || ready ? 0 : 600);
}

async function runProbe() {
  if (isBusy) return;
  const url = urlEl.value.trim();
  if (!url) return;
  const seq = ++probeSeq;
  setQualityHint("Azonosítás…");
  try {
    const res = await fetch("/api/probe", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        url,
        resolution: resolutionEl.value,
        destRoot: destEl.value.trim(),
      }),
    });
    const data = await res.json();
    if (seq !== probeSeq || url !== urlEl.value.trim()) return;
    probeCache = {
      url,
      heights: data.heights || null,
      playlist: !!data.playlist,
      needLogin: !!data.needLogin,
      message: data.message || "",
      thumbnail: data.thumbnail || "",
    };
    if (data.playlist && data.message) appendLog(data.message);
    if (!data.ok && selectedFormat() === "mp4" && !data.message) {
      setQualityHint("A felbontást nem sikerült ellenőrizni.");
    }
    applyProbeCache();
  } catch (_) {
    if (seq !== probeSeq) return;
    setCover("");
    if (selectedFormat() === "mp4") setQualityHint("A felbontást nem sikerült ellenőrizni.");
  }
}

function syncAccountButtons() {
  loginBtn.disabled = isBusy;
  loginDoneBtn.disabled = isBusy;
}

function rememberDest(path) {
  if (path) {
    try {
      localStorage.setItem(DEST_KEY, path);
    } catch (_) {}
  }
}

function folderName(path) {
  if (!path) return "";
  const parts = path.replace(/\\/g, "/").split("/").filter(Boolean);
  return parts[parts.length - 1] || path;
}

async function refreshStatus() {
  const res = await fetch("/api/status");
  const data = await res.json();
  $("title").textContent = `${data.title} ${data.version}`;
  if (!destEl.value) {
    let saved = "";
    try {
      saved = localStorage.getItem(DEST_KEY) || "";
    } catch (_) {}
    destEl.value = saved || data.downloads;
  }
  const auth = data.authOk
    ? `YouTube: belépve (${data.authBrowser || "?"})`
    : "YouTube: nincs mentett belépés";
  toolsEl.textContent =
    `yt-dlp: ${data.ytDlpOk ? "OK" : "HIÁNYZIK"}  ·  ffmpeg: ${data.ffmpegOk ? "OK" : "HIÁNYZIK"}  ·  ${auth}\n` +
    `${data.ytDlpPath}\n${data.ffmpegPath}`;
  setBusy(!!data.busy);
  if (!data.busy && !lastFolder) {
    setDlUI({ state: "Készen áll", path: "—", mode: "", enableOpen: false });
  }
  return data;
}

async function shutdown(force) {
  quitting = true;
  setDlUI({ state: "Kilépés…" });
  try {
    const res = await fetch("/api/shutdown", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ force: !!force }),
    });
    const data = await res.json();
    if (data.busy && !force) {
      quitting = false;
      const ok = confirm(
        "Letöltés folyamatban.\n\nBiztosan kilépsz? A letöltés megszakad."
      );
      if (ok) return shutdown(true);
      setDlUI({ state: "Kilépés megszakítva." });
      return;
    }
    setDlUI({ state: "Az alkalmazás leáll…" });
  } catch (_) {
    setDlUI({ state: "Az alkalmazás leállt." });
  }
}

document.querySelectorAll('input[name="format"]').forEach((el) => {
  el.addEventListener("change", syncFormatOptions);
});
urlEl.addEventListener("input", () => {
  probeCache = null;
  setCover("");
  scheduleProbe(false);
});
loginBtn.addEventListener("click", async () => {
  loginBtn.disabled = true;
  setDlUI({ state: "YouTube belépés ellenőrzése…" });
  try {
    const res = await fetch("/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    const data = await res.json();
    if (!res.ok || !data.ok) {
      setDlUI({ state: data.error || "A belépés nem sikerült.", mode: "fail" });
      syncAccountButtons();
      return;
    }
    if (data.alreadyLoggedIn || data.imported) {
      loginDoneBtn.hidden = true;
      const how = data.imported ? "átvéve a böngészőprofilból" : "már mentve";
      setDlUI({
        state: `YouTube belépés ${how}${data.browser ? " (" + data.browser + ")" : ""}.`,
        mode: "done",
      });
      if (data.videoUrl) urlEl.value = data.videoUrl;
      probeCache = null;
      scheduleProbe(true);
      syncAccountButtons();
      return;
    }
    if (data.needFinish || data.already) {
      loginDoneBtn.hidden = false;
      setDlUI({
        state: "Jelentkezz be a megnyílt ablakban, majd kattints a Belépés kész gombra.",
      });
    } else {
      loginDoneBtn.hidden = true;
      setDlUI({ state: "YouTube belépés kész.", mode: "done" });
      probeCache = null;
      scheduleProbe(true);
    }
  } catch (err) {
    setDlUI({ state: "Belépés hiba: " + err, mode: "fail" });
  }
  syncAccountButtons();
});

loginDoneBtn.addEventListener("click", async () => {
  loginDoneBtn.disabled = true;
  setDlUI({ state: "Belépés mentése…" });
  try {
    const res = await fetch("/api/login/finish", { method: "POST" });
    const data = await res.json();
    if (!res.ok || !data.ok) {
      setDlUI({ state: data.error || "A belépést nem sikerült menteni.", mode: "fail" });
      loginDoneBtn.disabled = false;
      return;
    }
    loginDoneBtn.hidden = true;
    if (data.videoUrl) {
      urlEl.value = data.videoUrl;
      setDlUI({ state: "Belépés mentve, a videó címe beírva.", mode: "done" });
    } else {
      setDlUI({ state: "YouTube belépés mentve.", mode: "done" });
    }
    probeCache = null;
    scheduleProbe(true);
  } catch (err) {
    setDlUI({ state: "Belépés hiba: " + err, mode: "fail" });
    loginDoneBtn.disabled = false;
  }
});

pasteUrlBtn.addEventListener("click", async () => {
  try {
    let text = "";
    if (navigator.clipboard && navigator.clipboard.readText) {
      text = await navigator.clipboard.readText();
    } else {
      // Tartalék régebbi környezethez
      urlEl.focus();
      const ok = document.execCommand("paste");
      if (ok) {
        setDlUI({ state: "Link beillesztve." });
        return;
      }
      throw new Error("clipboard API unavailable");
    }
    text = (text || "").trim();
    if (!text) {
      setDlUI({ state: "A vágólap üres." });
      return;
    }
    urlEl.value = text;
    urlEl.focus();
    setDlUI({ state: "Link beillesztve a vágólapról." });
    probeCache = null;
    scheduleProbe(true);
  } catch (err) {
    setDlUI({ state: "Vágólap olvasás nem engedélyezett." });
    appendLog("Vágólap hiba: " + err);
  }
});

browseBtn.addEventListener("click", async () => {
  setDlUI({ state: "Mappa kiválasztása…" });
  browseBtn.disabled = true;
  try {
    const res = await fetch("/api/pick-folder", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ initial: destEl.value.trim() }),
    });
    const data = await res.json();
    if (data.ok && data.path) {
      destEl.value = data.path;
      rememberDest(data.path);
      setDlUI({ state: "Mentési mappa beállítva." });
    } else if (data.error && data.error !== "cancelled" && !String(data.error).includes("exit status")) {
      setDlUI({ state: "Tallózás hiba" });
      appendLog("Tallózás hiba: " + data.error);
    } else {
      setDlUI({ state: "Nincs mappa kiválasztva." });
    }
  } catch (err) {
    setDlUI({ state: "Tallózás hiba" });
    appendLog("Tallózás hiba: " + err);
  } finally {
    browseBtn.disabled = false;
  }
});

openFolderBtn.addEventListener("click", async () => {
  if (!lastFolder) {
    setDlUI({ state: "Nincs megnyitható mappa" });
    return;
  }
  openFolderBtn.disabled = true;
  setDlUI({ state: "Mappa megnyitása…" });
  try {
    const res = await fetch("/api/open-folder", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: lastFolder }),
    });
    const text = await res.text();
    let data = {};
    try {
      data = JSON.parse(text);
    } catch (_) {}
    if (!res.ok) {
      appendLog("Mappa megnyitás hiba: " + text);
      setDlUI({ state: "Mappa megnyitás sikertelen" });
    } else {
      setDlUI({
        state: "Kész · " + folderName(lastFolder),
        path: lastFolder,
        mode: "done",
        enableOpen: true,
      });
    }
  } catch (err) {
    appendLog("Mappa megnyitás hiba: " + err);
    setDlUI({ state: "Mappa megnyitás sikertelen" });
  } finally {
    openFolderBtn.disabled = !lastFolder;
  }
});

downloadBtn.addEventListener("click", async () => {
  const url = urlEl.value.trim();
  if (!url) {
    setDlUI({ state: "Add meg a YouTube linket." });
    return;
  }
  const dest = destEl.value.trim();
  if (!dest) {
    setDlUI({ state: "Add meg a mentési mappát." });
    return;
  }
  rememberDest(dest);
  lastFolder = "";
  setBusy(true);
  setDlUI({
    state: "Folyamatban…",
    path: dest,
    mode: "active",
    enableOpen: false,
  });
  const res = await fetch("/api/download", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      url,
      format: selectedFormat(),
      bitrate: bitrateEl.value,
      resolution: resolutionEl.value,
      destRoot: dest,
    }),
  });
  if (!res.ok) {
    setBusy(false);
    setDlUI({
      state: "Nem indítható",
      mode: "fail",
      enableOpen: false,
    });
    appendLog("Hiba: " + (await res.text()));
  }
});

cancelBtn.addEventListener("click", async () => {
  await fetch("/api/cancel", { method: "POST" });
  setDlUI({ state: "Megszakítva…", mode: "fail" });
});

updateBtn.addEventListener("click", async () => {
  setBusy(true);
  setDlUI({ state: "yt-dlp frissítése…", mode: "active", enableOpen: false });
  await fetch("/api/update", { method: "POST" });
});

quitBtn.addEventListener("click", () => {
  shutdown(true);
});

window.addEventListener("beforeunload", (ev) => {
  if (quitting) return;
  if (isBusy) {
    ev.preventDefault();
    ev.returnValue = "";
  }
});

const es = new EventSource("/api/logs");
es.onmessage = (ev) => {
  let msg = "";
  try {
    msg = JSON.parse(ev.data);
  } catch {
    msg = ev.data;
  }
  if (msg.startsWith("EVENT:OK:")) {
    const folder = msg.slice("EVENT:OK:".length);
    lastFolder = folder;
    setBusy(false);
    setDlUI({
      state: "Kész · " + folderName(folder),
      path: folder,
      mode: "done",
      enableOpen: true,
    });
    appendLog("Sikeres: " + folder);
    return;
  }
  if (msg === "EVENT:FAIL" || msg === "EVENT:CANCEL") {
    setBusy(false);
    setDlUI({
      state: msg === "EVENT:CANCEL" ? "Megszakítva" : "Hiba a letöltés során",
      mode: "fail",
      enableOpen: !!lastFolder,
    });
    return;
  }
  if (msg.startsWith("Mappa: ")) {
    setDlUI({ path: msg.slice("Mappa: ".length) });
  }
  const barMsg = msg.split("\t");
  if (isProgressBar(barMsg[0])) {
    const pct = parseFloat(barMsg[2]);
    const eta = trackEta(pct, (barMsg[0].match(/ ETA (\S+)$/) || [])[1]);
    const bits = [];
    if (Number.isFinite(pct)) bits.push(Math.round(pct) + "%");
    if (barMsg[1]) bits.push(barMsg[1]);
    if (eta) bits.push("ETA " + eta);
    setDlUI({
      state: bits.join(" · ") || "Letöltés…",
      percent: Number.isFinite(pct) ? pct : undefined,
      mode: "active",
    });
    setProgressLog(barMsg[0]);
    return;
  }
  const pct = applyDownloadLine(msg);
  if (pct != null) {
    const eta = trackEta(pct, (msg.match(/\bETA\s+(\S+)/) || [])[1]);
    let bar = progressBar(pct);
    if (eta) bar += " ETA " + eta;
    setProgressLog(bar);
    return;
  }
  if (!msg.trim()) return;
  if (msg.includes("yt-dlp frissítés kész") || msg.includes("yt-dlp frissítés hibakód")) {
    setBusy(false);
    setDlUI({
      state: msg.includes("kész") ? "yt-dlp frissítve" : "Frissítés sikertelen",
      mode: msg.includes("kész") ? "done" : "fail",
      enableOpen: !!lastFolder,
    });
  }
  appendLog(msg);
};

syncFormatOptions();
refreshStatus().catch((err) => {
  setDlUI({ state: "Státusz hiba: " + err, mode: "fail" });
});
