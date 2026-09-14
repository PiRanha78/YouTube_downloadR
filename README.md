# YouTube DownloadR – GUI

Egységes Windows GUI a három `.bat` letöltő helyett (`yt-dlp` + `ffmpeg`).

## Funkciók

- **MP3** – választható bitráta: 128 / 256 / 320 kbps  
- **MP4** – mindig a legmagasabb elérhető minőség (`bestvideo+bestaudio` → mp4)  
- **WAV** – veszteségmentes hang  
- **Külön mappa minden mentéshez** – `downloads/<cím> [<videó-id>]/`  
- Relatív útvonalak (nem kell `D:\YouTube_downloads\...`)  
- yt-dlp frissítés gomb a GUI-ból  

## Könyvtárstruktúra

```
YouTube_DownloadR/
├── YouTube_DownloadR.exe   ← a build után
├── ytd_gui.py              ← forrás (fejlesztéshez)
├── build_exe.bat           ← .exe készítő
├── app/
│   ├── yt-dlp.exe
│   └── ffmpeg-8.0.1-essentials_build/
│       └── bin/
│           └── ffmpeg.exe
└── downloads/              ← alapértelmezett mentési gyökér
```

## Kész .exe csomag

```
release/YouTube_DownloadR/YouTube_DownloadR.exe
```

Egyetlen fájl (~93 MB): a `yt-dlp` + `ffmpeg` + `ffprobe` be van ágyazva.  
Natív Windows ablakban nyílik (Microsoft Edge WebView2, nem böngészőlap).  
Első indításkor kibontja a segédprogramokat ide: `%LOCALAPPDATA%\YouTube_DownloadR\runtime\`  
Nincs külön telepítő, nincs `app\` mappa.

Ha az ablak nem jelenik meg, telepítsd a [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)-ot (Win10/11-en általában megvan).

### Újrafordítás (macOS / Linux)

Kell: [Go](https://go.dev/dl/)

```bash
./build_exe.sh
```

### Alternatíva: Python GUI (Windows)

```bat
python ytd_gui.py
build_exe.bat
```

(A Python build nem ágyazza be a 90+ MB segédprogramokat.)

## Régi .bat fájlok

A `yt-mp3-downloader.bat`, `yt-mp4-downloader.bat` és `yt-WAV-downloader.bat` továbbra is használhatók; a GUI ugyanazokat a segédprogramokat hívja, rugalmasabb beállításokkal.
