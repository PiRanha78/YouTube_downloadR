# YouTube DownloadR

Egységes GUI a `.bat` letöltők helyett (`yt-dlp` + `ffmpeg`).

## Funkciók

- **MP3** – bitráta: 128 / 256 / 320 kbps  
- **MP4** – 720p / 1080p / 2K (1440p) / 4K (2160p)  
- **WAV** – veszteségmentes hang  
- Mentési mappa tallózása + URL vágólapról beillesztés  
- Egyetlen videó: közvetlenül a választott mappába  
- Playlist (`list=` a URL-ben): videónként külön alkönyvtár  
- Letöltési sáv + **Mappa megnyitás**  
- Beágyazott yt-dlp + ffmpeg (külön telepítés nélkül)

## Letöltés (Release)

A kész binárisok a [GitHub Releases](https://github.com/PiRanha78/YouTube_downloadR/releases) oldalon:

| Platform | Fájl |
|----------|------|
| Windows | `YouTube_DownloadR.exe` |
| macOS Intel | `YouTube_DownloadR_macOS_Intel.zip` |
| macOS Apple Silicon | `YouTube_DownloadR_macOS_AppleSilicon.zip` |

> Gatekeeper: jobb klikk → Megnyitás, vagy:  
> `xattr -dr com.apple.quarantine YouTube_DownloadR.app`

## Újrafordítás

Kell: [Go](https://go.dev/dl/), macOS-en Xcode Command Line Tools.

```bash
# Minden platform
./build_all.sh

# Egyenként
./build_exe.sh          # Windows
./build_mac.sh amd64    # Intel
./build_mac.sh arm64    # Apple Silicon
```

Darwin tool bundle frissítés:

```bash
./scripts/prepare_darwin_bundle.sh all
```

## Fejlesztés

```bash
python3 ytd_gui.py
```

## Régi .bat fájlok

`yt-mp3-downloader.bat`, `yt-mp4-downloader.bat`, `yt-WAV-downloader.bat` – relatív `app\` útvonalakkal.
