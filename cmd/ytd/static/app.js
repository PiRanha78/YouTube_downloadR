const $ = (id) => document.getElementById(id);

const urlEl = $("url");
const pasteUrlBtn = $("pasteUrl");
const destEl = $("dest");
const browseBtn = $("browse");
const bitrateEl = $("bitrate");
const bitrateRow = $("bitrateRow");
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

function selectedFormat() {
  return document.querySelector('input[name="format"]:checked').value;
}

function setDlUI({ state, path, mode, enableOpen }) {
  if (state != null) dlState.textContent = state;
  if (path != null) {
    dlPath.textContent = path || "—";
    dlPath.title = path || "";
  }
  dlTrack.classList.remove("active", "done", "fail");
  if (mode) dlTrack.classList.add(mode);
  if (enableOpen != null) {
    openFolderBtn.disabled = !enableOpen;
  }
}

function setBusy(busy) {
  isBusy = !!busy;
  downloadBtn.disabled = busy;
  updateBtn.disabled = busy;
  browseBtn.disabled = busy;
  pasteUrlBtn.disabled = busy;
  cancelBtn.disabled = !busy;
  if (busy) {
    openFolderBtn.disabled = true;
    setDlUI({ state: "Folyamatban…", mode: "active", enableOpen: false });
  }
}

function appendLog(line) {
  logEl.textContent += line + "\n";
  logEl.scrollTop = logEl.scrollHeight;
}

function syncBitrate() {
  const mp3 = selectedFormat() === "mp3";
  bitrateRow.classList.toggle("disabled", !mp3);
  bitrateEl.disabled = !mp3;
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
  toolsEl.textContent =
    `yt-dlp: ${data.ytDlpOk ? "OK" : "HIÁNYZIK"}  ·  ffmpeg: ${data.ffmpegOk ? "OK" : "HIÁNYZIK"}\n` +
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
  el.addEventListener("change", syncBitrate);
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
  if (isBusy) {
    const ok = confirm(
      "Letöltés folyamatban.\n\nBiztosan kilépsz? A letöltés megszakad."
    );
    if (!ok) return;
    shutdown(true);
    return;
  }
  if (!confirm("Bezárod a YouTube DownloadR alkalmazást?")) return;
  shutdown(false);
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

syncBitrate();
refreshStatus().catch((err) => {
  setDlUI({ state: "Státusz hiba: " + err, mode: "fail" });
});
