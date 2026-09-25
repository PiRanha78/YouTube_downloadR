#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
YouTube DownloadR – egységes GUI a yt-dlp + ffmpeg alapú letöltéshez.
Formátumok: MP3 (128/256/320 kbps), MP4 (720p / 1080p / 2K / 4K), WAV.
Minden mentés külön könyvtárba kerül.
"""

from __future__ import annotations

import os
import subprocess
import sys
import threading
import tkinter as tk
from datetime import datetime
from pathlib import Path
from tkinter import filedialog, messagebox, ttk
from urllib.parse import parse_qs, urlparse


APP_TITLE = "YouTube DownloadR"
APP_VERSION = "1.0.0"


def js_runtime_args() -> list[str]:
    candidates = [
        Path(os.environ.get("ProgramFiles", "")) / "nodejs" / "node.exe",
        Path(os.environ.get("LOCALAPPDATA", "")) / "Programs" / "nodejs" / "node.exe",
    ]
    for path in candidates:
        if path.is_file():
            return ["--js-runtimes", f"node:{path}", "--remote-components", "ejs:github"]
    return []


def cookies_file_path(browser: str) -> Path:
    local = os.environ.get("LOCALAPPDATA")
    if local:
        root = Path(local)
    else:
        root = Path.home() / ".local" / "share"
    return root / "YouTube_DownloadR" / "auth" / browser / "cookies.txt"


def app_base_dir() -> Path:
    """Futtatható / script mappája (PyInstaller .exe esetén is)."""
    if getattr(sys, "frozen", False):
        return Path(sys.executable).resolve().parent
    return Path(__file__).resolve().parent


def find_yt_dlp(base: Path) -> Path | None:
    candidates = [
        base / "app" / "yt-dlp.exe",
        base / "yt-dlp.exe",
        base / "app" / "yt-dlp",
        base / "yt-dlp",
    ]
    for path in candidates:
        if path.is_file():
            return path
    return None


def find_ffmpeg(base: Path) -> Path | None:
    candidates = [
        base / "app" / "ffmpeg-8.0.1-essentials_build" / "bin" / "ffmpeg.exe",
        base / "app" / "ffmpeg" / "bin" / "ffmpeg.exe",
        base / "app" / "ffmpeg.exe",
        base / "ffmpeg.exe",
        base / "app" / "ffmpeg-8.0.1-essentials_build" / "bin" / "ffmpeg",
        base / "app" / "ffmpeg" / "bin" / "ffmpeg",
    ]
    for path in candidates:
        if path.is_file():
            return path
    # Bármely ffmpeg-*-essentials_build/bin/ffmpeg.exe
    for match in sorted((base / "app").glob("ffmpeg-*/bin/ffmpeg.exe")):
        if match.is_file():
            return match
    for match in sorted((base / "app").glob("ffmpeg-*/bin/ffmpeg")):
        if match.is_file():
            return match
    return None


def is_playlist_url(url: str) -> bool:
    try:
        q = parse_qs(urlparse(url).query)
        return bool((q.get("list") or [""])[0].strip())
    except Exception:
        return "list=" in url.lower()


class DownloadApp(tk.Tk):
    def __init__(self) -> None:
        super().__init__()
        self.title(f"{APP_TITLE} {APP_VERSION}")
        self.minsize(640, 520)
        self.geometry("720x580")
        self.configure(bg="#1a1d23")

        self.base = app_base_dir()
        self.yt_dlp = find_yt_dlp(self.base)
        self.ffmpeg = find_ffmpeg(self.base)
        self.download_root = tk.StringVar(value=str(self.base / "downloads"))
        self.format_var = tk.StringVar(value="mp3")
        self.bitrate_var = tk.StringVar(value="320")
        self.resolution_var = tk.StringVar(value="1080p")
        self.cookies_var = tk.StringVar(value="Chrome")
        self.url_var = tk.StringVar()
        self.status_var = tk.StringVar(value="Készen áll.")
        self._busy = False
        self._proc: subprocess.Popen | None = None

        self._build_style()
        self._build_ui()
        self._refresh_tool_status()

    def _build_style(self) -> None:
        style = ttk.Style(self)
        try:
            style.theme_use("clam")
        except tk.TclError:
            pass
        bg = "#1a1d23"
        panel = "#242830"
        fg = "#e8eaed"
        accent = "#3d8bfd"
        muted = "#9aa0a6"
        style.configure(".", background=bg, foreground=fg, fieldbackground=panel)
        style.configure("TFrame", background=bg)
        style.configure("Card.TFrame", background=panel)
        style.configure("TLabel", background=bg, foreground=fg, font=("Segoe UI", 10))
        style.configure("Muted.TLabel", background=bg, foreground=muted, font=("Segoe UI", 9))
        style.configure("Card.TLabel", background=panel, foreground=fg, font=("Segoe UI", 10))
        style.configure("Title.TLabel", background=bg, foreground=fg, font=("Segoe UI", 16, "bold"))
        style.configure("TRadiobutton", background=panel, foreground=fg, font=("Segoe UI", 10))
        style.configure("TButton", font=("Segoe UI", 10), padding=6)
        style.configure("Accent.TButton", font=("Segoe UI", 11, "bold"), padding=(14, 8))
        style.map(
            "Accent.TButton",
            background=[("!disabled", accent), ("disabled", "#3a3f4b")],
            foreground=[("!disabled", "#ffffff"), ("disabled", "#888")],
        )
        style.configure("TEntry", fieldbackground=panel, foreground=fg, insertcolor=fg)
        style.configure("TCombobox", fieldbackground=panel, foreground=fg, background=panel)
        style.configure("Horizontal.TProgressbar", troughcolor="#2c313a", background=accent)

    def _build_ui(self) -> None:
        pad = {"padx": 16, "pady": 8}
        root = ttk.Frame(self)
        root.pack(fill=tk.BOTH, expand=True)

        ttk.Label(root, text=APP_TITLE, style="Title.TLabel").pack(anchor=tk.W, **pad)
        ttk.Label(
            root,
            text="YouTube videó / hang letöltése · yt-dlp + ffmpeg",
            style="Muted.TLabel",
        ).pack(anchor=tk.W, padx=16, pady=(0, 4))

        # URL
        url_row = ttk.Frame(root)
        url_row.pack(fill=tk.X, **pad)
        ttk.Label(url_row, text="YouTube URL").pack(anchor=tk.W)
        url_inner = ttk.Frame(url_row)
        url_inner.pack(fill=tk.X, pady=(4, 0))
        entry = ttk.Entry(url_inner, textvariable=self.url_var, font=("Segoe UI", 11))
        entry.pack(side=tk.LEFT, fill=tk.X, expand=True)
        ttk.Button(url_inner, text="📋", width=3, command=self._paste_url).pack(
            side=tk.LEFT, padx=(8, 0)
        )
        entry.focus_set()

        # Formátum kártya
        card = ttk.Frame(root, style="Card.TFrame")
        card.pack(fill=tk.X, padx=16, pady=8)
        inner = ttk.Frame(card, style="Card.TFrame")
        inner.pack(fill=tk.X, padx=12, pady=12)

        ttk.Label(inner, text="Formátum", style="Card.TLabel").grid(row=0, column=0, sticky=tk.W)

        fmt_frame = ttk.Frame(inner, style="Card.TFrame")
        fmt_frame.grid(row=1, column=0, columnspan=2, sticky=tk.W, pady=(6, 10))

        ttk.Radiobutton(
            fmt_frame, text="MP3", variable=self.format_var, value="mp3",
            command=self._on_format_change, style="TRadiobutton",
        ).pack(side=tk.LEFT, padx=(0, 16))
        ttk.Radiobutton(
            fmt_frame, text="MP4", variable=self.format_var, value="mp4",
            command=self._on_format_change, style="TRadiobutton",
        ).pack(side=tk.LEFT, padx=(0, 16))
        ttk.Radiobutton(
            fmt_frame, text="WAV", variable=self.format_var, value="wav",
            command=self._on_format_change, style="TRadiobutton",
        ).pack(side=tk.LEFT)

        self.bitrate_label = ttk.Label(inner, text="MP3 bitráta", style="Card.TLabel")
        self.bitrate_label.grid(row=2, column=0, sticky=tk.W)
        self.bitrate_combo = ttk.Combobox(
            inner,
            textvariable=self.bitrate_var,
            values=("128", "256", "320"),
            state="readonly",
            width=8,
        )
        self.bitrate_combo.grid(row=2, column=1, sticky=tk.W, padx=(12, 0))
        self.bitrate_combo.current(2)

        self.resolution_label = ttk.Label(inner, text="MP4 felbontás", style="Card.TLabel")
        self.resolution_label.grid(row=3, column=0, sticky=tk.W, pady=(8, 0))
        self.resolution_combo = ttk.Combobox(
            inner,
            textvariable=self.resolution_var,
            values=("720p", "1080p", "2K (1440p)", "4K (2160p)"),
            state="disabled",
            width=16,
        )
        self.resolution_combo.grid(row=3, column=1, sticky=tk.W, padx=(12, 0), pady=(8, 0))
        self._resolution_heights = {
            "720p": "720",
            "1080p": "1080",
            "2K (1440p)": "1440",
            "4K (2160p)": "2160",
        }

        self.cookies_label = ttk.Label(inner, text="YouTube fiók", style="Card.TLabel")
        self.cookies_label.grid(row=4, column=0, sticky=tk.W, pady=(8, 0))
        self.cookies_combo = ttk.Combobox(
            inner,
            textvariable=self.cookies_var,
            values=("Chrome", "Brave", "Nincs"),
            state="readonly",
            width=16,
        )
        self.cookies_combo.grid(row=4, column=1, sticky=tk.W, padx=(12, 0), pady=(8, 0))
        self._cookies_browsers = {
            "Chrome": "chrome",
            "Brave": "brave",
            "Nincs": "",
        }

        # Mentési mappa
        dest_row = ttk.Frame(root)
        dest_row.pack(fill=tk.X, **pad)
        ttk.Label(dest_row, text="Mentési gyökérmappa").pack(anchor=tk.W)
        dest_inner = ttk.Frame(dest_row)
        dest_inner.pack(fill=tk.X, pady=(4, 0))
        ttk.Entry(dest_inner, textvariable=self.download_root).pack(
            side=tk.LEFT, fill=tk.X, expand=True
        )
        ttk.Button(dest_inner, text="Tallózás…", command=self._browse_dest).pack(
            side=tk.LEFT, padx=(8, 0)
        )
        ttk.Label(
            dest_row,
            text="Egyetlen videó közvetlenül ide mentődik. Playlist (list= a linkben): videónként külön mappa.",
            style="Muted.TLabel",
        ).pack(anchor=tk.W, pady=(4, 0))

        # Gombok
        btn_row = ttk.Frame(root)
        btn_row.pack(fill=tk.X, **pad)
        self.download_btn = ttk.Button(
            btn_row, text="Letöltés", style="Accent.TButton", command=self._start_download
        )
        self.download_btn.pack(side=tk.LEFT)
        self.cancel_btn = ttk.Button(
            btn_row, text="Megszakítás", command=self._cancel_download, state=tk.DISABLED
        )
        self.cancel_btn.pack(side=tk.LEFT, padx=(8, 0))
        ttk.Button(btn_row, text="yt-dlp frissítés", command=self._update_ytdlp).pack(
            side=tk.RIGHT
        )

        self.progress = ttk.Progressbar(root, mode="indeterminate")
        self.progress.pack(fill=tk.X, padx=16, pady=(0, 4))

        ttk.Label(root, textvariable=self.status_var, style="Muted.TLabel").pack(
            anchor=tk.W, padx=16
        )

        # Napló
        log_frame = ttk.Frame(root)
        log_frame.pack(fill=tk.BOTH, expand=True, padx=16, pady=(4, 16))
        self.log = tk.Text(
            log_frame,
            height=12,
            wrap=tk.WORD,
            bg="#12141a",
            fg="#c8ccd2",
            insertbackground="#c8ccd2",
            font=("Consolas", 9),
            relief=tk.FLAT,
            borderwidth=0,
        )
        scroll = ttk.Scrollbar(log_frame, command=self.log.yview)
        self.log.configure(yscrollcommand=scroll.set)
        self.log.pack(side=tk.LEFT, fill=tk.BOTH, expand=True)
        scroll.pack(side=tk.RIGHT, fill=tk.Y)

        self.tool_label = ttk.Label(root, text="", style="Muted.TLabel")
        self.tool_label.pack(anchor=tk.W, padx=16, pady=(0, 12))

        self._on_format_change()

    def _on_format_change(self) -> None:
        fmt = self.format_var.get()
        is_mp3 = fmt == "mp3"
        is_mp4 = fmt == "mp4"
        self.bitrate_combo.configure(state="readonly" if is_mp3 else "disabled")
        self.bitrate_label.configure(foreground="#e8eaed" if is_mp3 else "#5f6368")
        self.resolution_combo.configure(state="readonly" if is_mp4 else "disabled")
        self.resolution_label.configure(foreground="#e8eaed" if is_mp4 else "#5f6368")

    def _browse_dest(self) -> None:
        path = filedialog.askdirectory(
            title="Mentési mappa",
            initialdir=self.download_root.get() or str(self.base),
        )
        if path:
            self.download_root.set(path)

    def _paste_url(self) -> None:
        try:
            text = self.clipboard_get().strip()
        except tk.TclError:
            self.status_var.set("A vágólap üres vagy nem olvasható.")
            return
        if not text:
            self.status_var.set("A vágólap üres.")
            return
        self.url_var.set(text)
        self.status_var.set("Link beillesztve a vágólapról.")

    def _log(self, text: str) -> None:
        self.log.insert(tk.END, text + "\n")
        self.log.see(tk.END)

    def _set_busy(self, busy: bool) -> None:
        self._busy = busy
        self.download_btn.configure(state=tk.DISABLED if busy else tk.NORMAL)
        self.cancel_btn.configure(state=tk.NORMAL if busy else tk.DISABLED)
        if busy:
            self.progress.start(12)
        else:
            self.progress.stop()

    def _refresh_tool_status(self) -> None:
        yt = "OK" if self.yt_dlp else "HIÁNYZIK"
        ff = "OK" if self.ffmpeg else "HIÁNYZIK"
        yt_path = self.yt_dlp if self.yt_dlp else (self.base / "app" / "yt-dlp.exe")
        ff_path = self.ffmpeg if self.ffmpeg else (
            self.base / "app" / "ffmpeg-8.0.1-essentials_build" / "bin" / "ffmpeg.exe"
        )
        self.tool_label.configure(
            text=f"yt-dlp: {yt}  ·  ffmpeg: {ff}\n{yt_path}\n{ff_path}"
        )

    def _ensure_tools(self) -> bool:
        self.yt_dlp = find_yt_dlp(self.base)
        self.ffmpeg = find_ffmpeg(self.base)
        self._refresh_tool_status()
        if not self.yt_dlp or not self.ffmpeg:
            messagebox.showerror(
                "Hiányzó segédprogram",
                "Nem található a yt-dlp.exe vagy az ffmpeg.exe.\n\n"
                "Várt elhelyezés:\n"
                f"  {self.base / 'app' / 'yt-dlp.exe'}\n"
                f"  {self.base / 'app' / 'ffmpeg-8.0.1-essentials_build' / 'bin' / 'ffmpeg.exe'}",
            )
            return False
        return True

    def _start_download(self) -> None:
        if self._busy:
            return
        url = self.url_var.get().strip()
        if not url:
            messagebox.showwarning("Hiányzó URL", "Add meg a YouTube videó linkjét.")
            return
        if not self._ensure_tools():
            return

        dest_root = Path(self.download_root.get().strip() or (self.base / "downloads"))
        try:
            dest_root.mkdir(parents=True, exist_ok=True)
        except OSError as exc:
            messagebox.showerror("Mappa hiba", f"Nem lehet létrehozni a mappát:\n{exc}")
            return

        fmt = self.format_var.get()
        bitrate = self.bitrate_var.get()
        resolution = self.resolution_var.get()
        extra = ""
        if fmt == "mp3":
            extra = f" @ {bitrate} kbps"
        elif fmt == "mp4":
            extra = " @ " + (resolution if resolution in self._resolution_heights else "1080p")
        cookies = self._cookies_browsers.get(self.cookies_var.get(), "")
        if cookies and not cookies_file_path(cookies).is_file():
            messagebox.showwarning(
                "Nincs YouTube belépés",
                "Nincs mentett belépés ehhez a böngészőhöz.\n\n"
                "A Go alkalmazásban kattints a Bejelentkezés gombra, "
                "jelentkezz be, majd a Belépés kész gombra.",
            )
            return
        self._set_busy(True)
        self.status_var.set("Letöltés folyamatban…")
        self._log(f"——— {datetime.now().strftime('%Y-%m-%d %H:%M:%S')} ———")
        self._log(f"URL: {url}")
        self._log(f"Formátum: {fmt.upper()}{extra}")
        if cookies:
            self._log(f"YouTube fiók: mentett sütik ({cookies})")

        threading.Thread(
            target=self._download_worker,
            args=(url, dest_root, fmt, bitrate, resolution, cookies),
            daemon=True,
        ).start()

    def _cancel_download(self) -> None:
        if self._proc and self._proc.poll() is None:
            self._proc.terminate()
            self._log("Megszakítás kérve…")
            self.status_var.set("Megszakítva.")

    def _update_ytdlp(self) -> None:
        if self._busy:
            return
        if not self._ensure_tools():
            return
        self._set_busy(True)
        self.status_var.set("yt-dlp frissítése…")
        self._log("yt-dlp frissítés indul…")

        def worker() -> None:
            code = self._run_cmd([str(self.yt_dlp), "-U"])
            self.after(0, lambda: self._on_update_done(code))

        threading.Thread(target=worker, daemon=True).start()

    def _on_update_done(self, code: int) -> None:
        self._set_busy(False)
        if code == 0:
            self.status_var.set("yt-dlp frissítve.")
            self._log("yt-dlp frissítés kész.")
        else:
            self.status_var.set("Frissítés sikertelen.")
            self._log(f"yt-dlp frissítés hibakód: {code}")

    def _build_yt_cmd(
        self,
        url: str,
        out_dir: Path,
        fmt: str,
        bitrate: str,
        resolution: str,
        cookies_browser: str,
        playlist: bool,
    ) -> list[str]:
        assert self.yt_dlp is not None and self.ffmpeg is not None
        ffmpeg_loc = str(self.ffmpeg)
        if playlist:
            out_tpl = str(
                out_dir / "%(playlist)s" / "%(title)s [%(id)s]" / "%(title)s.%(ext)s"
            )
        else:
            out_tpl = str(out_dir / "%(title)s.%(ext)s")
        common = [
            str(self.yt_dlp),
            "--yes-playlist" if playlist else "--no-playlist",
            "--ffmpeg-location",
            ffmpeg_loc,
            "--force-ipv4",
            "-o",
            out_tpl,
        ]
        if cookies_browser in {"chrome", "brave"}:
            cookie_file = cookies_file_path(cookies_browser)
            if cookie_file.is_file():
                common += ["--cookies", str(cookie_file)]
        common += js_runtime_args()
        common += ["--extractor-args", "youtube:player_client=default,web_safari"]

        if fmt == "mp4":
            height = self._resolution_heights.get(resolution, "1080")
            selector = (
                f"bestvideo*[height<={height}]+bestaudio/best[height<={height}]/best"
            )
            return common + [
                "-f",
                selector,
                "--merge-output-format",
                "mp4",
                url,
            ]

        if fmt == "wav":
            return common + [
                "-ciw",
                "-f",
                "bestaudio/best",
                "-x",
                "--audio-format",
                "wav",
                "--concurrent-fragments",
                "10",
                "--http-chunk-size",
                "10M",
                "--postprocessor-args",
                "ffmpeg:-c:a pcm_s16le -threads 4",
                url,
            ]

        # mp3
        br = bitrate if bitrate in {"128", "256", "320"} else "320"
        return common + [
            "-ciw",
            "-f",
            "bestaudio/best",
            "-x",
            "--audio-format",
            "mp3",
            "--audio-quality",
            f"{br}K",
            "--concurrent-fragments",
            "10",
            "--http-chunk-size",
            "10M",
            "--postprocessor-args",
            f"ffmpeg:-c:a libmp3lame -b:a {br}k -threads 4",
            url,
        ]

    def _download_worker(
        self, url: str, dest_root: Path, fmt: str, bitrate: str, resolution: str, cookies_browser: str
    ) -> None:
        playlist = is_playlist_url(url)
        out_dir = dest_root
        try:
            out_dir.mkdir(parents=True, exist_ok=True)
        except OSError as exc:
            self.after(0, lambda: self._on_download_done(False, f"Mappa hiba: {exc}"))
            return

        if playlist:
            self.after(0, lambda: self._log("Playlist észlelve (list=) – videónként külön alkönyvtár."))
        else:
            self.after(0, lambda: self._log("Egyetlen videó – mentés közvetlenül a kiválasztott mappába."))

        self.after(0, lambda: self._log(f"Mappa: {out_dir}"))
        cmd = self._build_yt_cmd(url, out_dir, fmt, bitrate, resolution, cookies_browser, playlist)
        self.after(0, lambda: self._log("Parancs: " + " ".join(cmd)))
        code = self._run_cmd(cmd)
        if code == 0:
            self.after(0, lambda: self._on_download_done(True, str(out_dir)))
        else:
            self.after(
                0,
                lambda: self._on_download_done(False, f"Hibakód: {code}"),
            )

    def _run_cmd(self, cmd: list[str]) -> int:
        flags = 0
        if os.name == "nt":
            flags = getattr(subprocess, "CREATE_NO_WINDOW", 0)
        try:
            self._proc = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                encoding="utf-8",
                errors="replace",
                creationflags=flags,
            )
        except OSError as exc:
            self.after(0, lambda: self._log(f"Indítási hiba: {exc}"))
            return 1

        assert self._proc.stdout is not None
        for line in self._proc.stdout:
            line = line.rstrip("\n")
            if line:
                self.after(0, lambda L=line: self._log(L))
        return self._proc.wait()

    def _on_download_done(self, ok: bool, detail: str) -> None:
        self._set_busy(False)
        self._proc = None
        if ok:
            self.status_var.set("Kész.")
            self._log(f"Sikeres letöltés → {detail}")
            messagebox.showinfo("Kész", f"A tartalom letöltése sikerült.\n\n{detail}")
        else:
            self.status_var.set("Hiba a letöltés során.")
            self._log(f"Sikertelen: {detail}")
            messagebox.showerror("Hiba", f"A letöltés nem sikerült.\n{detail}")


def main() -> None:
    app = DownloadApp()
    app.mainloop()


if __name__ == "__main__":
    main()
