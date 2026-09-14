@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

echo ============================================
echo  YouTube DownloadR - .exe epitse
echo ============================================
echo.

where python >nul 2>&1
if errorlevel 1 (
    echo HIBA: Python nem talalhato. Telepitsd: https://www.python.org/downloads/
    echo Pip telepiteskor jelold be: "Add python.exe to PATH"
    pause
    exit /b 1
)

echo [1/3] Fuggosegek telepitese...
python -m pip install --upgrade pip
python -m pip install -r requirements.txt
if errorlevel 1 (
    echo HIBA: pip telepites sikertelen.
    pause
    exit /b 1
)

echo.
echo [2/3] PyInstaller futtatasa...
if exist build rmdir /s /q build
if exist dist\YouTube_DownloadR.exe del /f /q dist\YouTube_DownloadR.exe

python -m PyInstaller --noconfirm --clean ^
  --onefile ^
  --windowed ^
  --name "YouTube_DownloadR" ^
  --distpath dist ^
  --workpath build ^
  --specpath build ^
  ytd_gui.py

if errorlevel 1 (
    echo HIBA: az .exe epitse sikertelen.
    pause
    exit /b 1
)

echo.
echo [3/3] Csomag osszeallitasa...
set "OUT=release\YouTube_DownloadR"
if exist "%OUT%" rmdir /s /q "%OUT%"
mkdir "%OUT%"
mkdir "%OUT%\app"

copy /y "dist\YouTube_DownloadR.exe" "%OUT%\YouTube_DownloadR.exe" >nul

if exist "app\yt-dlp.exe" (
    copy /y "app\yt-dlp.exe" "%OUT%\app\yt-dlp.exe" >nul
) else (
    echo FIGYELEM: app\yt-dlp.exe hianyzik - masold be kesobb.
)

if exist "app\ffmpeg-8.0.1-essentials_build" (
    xcopy /e /i /y "app\ffmpeg-8.0.1-essentials_build" "%OUT%\app\ffmpeg-8.0.1-essentials_build" >nul
) else (
    echo FIGYELEM: ffmpeg mappa hianyzik - masold be kesobb.
)

echo.
echo Kesz!
echo Futtathato csomag:
echo   %CD%\%OUT%\YouTube_DownloadR.exe
echo.
echo A teljes "%OUT%" mappat masold at oda, ahol hasznalni szeretned.
echo Az app\ mappa (yt-dlp + ffmpeg) az .exe mellett kell legyen.
echo.
pause
