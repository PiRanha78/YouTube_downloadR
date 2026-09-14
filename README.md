# YouTube DownloadR

Egységes GUI a `.bat` letöltők helyett (`yt-dlp` + `ffmpeg`).

## Funkciók

- **MP3** – bitráta: 128 / 256 / 320 kbps  
- **MP4** – legmagasabb minőség  
- **WAV** – veszteségmentes hang  
- Mentési mappa tallózása  
- Minden letöltés külön alkönyvtárba  
- Letöltési sáv + **Mappa megnyitás**  
- Beágyazott yt-dlp + ffmpeg (külön telepítés nélkül)

## Letöltés

### Windows
`release/YouTube_DownloadR/YouTube_DownloadR.exe` (~93 MB)  
Natív ablak (Edge WebView2).

### macOS (Intel)
`release/macOS/YouTube_DownloadR.app`  
Natív ablak (WebKit). Első indításkor a segédprogramok ide kerülnek:  
`~/Library/Application Support/YouTube_DownloadR/runtime/`  
Alapértelmezett mentési gyökér: `~/Documents/YouTube_DownloadR/downloads`

> Gatekeeper: ha blokkolja, jobb klikk → Megnyitás, vagy:  
> `xattr -dr com.apple.quarantine /útvonal/YouTube_DownloadR.app`

## Újrafordítás

Kell: [Go](https://go.dev/dl/), macOS-en Xcode Command Line Tools.

```bash
# Windows .exe (bármely OS-ről, ha megvan az app/ windows bináris)
./build_exe.sh

# macOS Intel .app (csak Macen)
./scripts/prepare_darwin_bundle.sh   # egyszer / frissítéskor
./build_mac.sh
```

## Fejlesztés

```bash
# Python GUI (opcionális)
python3 ytd_gui.py
```

## Régi .bat fájlok

`yt-mp3-downloader.bat`, `yt-mp4-downloader.bat`, `yt-WAV-downloader.bat` – relatív `app\` útvonalakkal.
