@echo off
chcp 1250 >nul
setlocal ENABLEDELAYEDEXPANSION

:: Relat�v �tvonalak a .bat mapp�j�hoz k�pest
set "ROOT=%~dp0"
set "ROOT=%ROOT:~0,-1%"
set "yt_dlp_path=%ROOT%\app\yt-dlp.exe"
set "ffmpeg_path=%ROOT%\app\ffmpeg-8.0.1-essentials_build\bin\ffmpeg.exe"

echo.
echo =====================================================================
echo Ez az  ingyenes  program  a Youtube platformr�l  ment  le hangs�vokat
echo WAV form�tumban.
echo A let�lt�shez  sz�ks�ges,  ingyenes  k�t seg�dprogramot itt  �red el:
echo.
echo (1) yt-dlp.exe
echo      Let�lthet�: https://github.com/yt-dlp/yt-dlp/releases
echo (2) ffmpeg.exe
echo      Mentsd le a ZIP csomagot,  csomagold ki, majd m�sold  a  hely�re
echo      a teljes mapp�t: ffmpeg-release-essentials.zip.
echo      Let�lthet�: https://www.gyan.dev/ffmpeg/builds/
echo.
echo A g�peden  ezen az  �tvonalon fogja keresni a  let�lt�  program a k�t
echo seg�dprogramot  (a .bat f�jl mapp�j�hoz k�pest):
echo.
echo     %yt_dlp_path%
echo     %ffmpeg_path%
echo =====================================================================
echo.
pause

if not exist "%yt_dlp_path%" (
    echo HIBA: Nem tal�lhat�: %yt_dlp_path%
    pause
    exit /b 1
)
if not exist "%ffmpeg_path%" (
    echo HIBA: Nem tal�lhat�: %ffmpeg_path%
    pause
    exit /b 1
)

echo===============================================
echo   Ellen�rz�m �s friss�tem a yt-dlp.exe f�jlt
echo===============================================
"%yt_dlp_path%" -U

:start
cls
echo ===================================================
echo        YOUTUBE VIDE� HANGANYAG�NAK LET�LT�SE
echo ================= 2025 m�rcius 27. ================
echo.
echo====================================================
echo                     K�sz�tette:
echo Ambrus Attila J�zsef (ambrusattilajozsef@gmail.com)
echo          Patreon: ambrusaalkotoistudioja
echo:      PayPal: paypal.me/mindenkinekvanhangja
echo                Youtube: @hangoskonyv
echo ===================================================
echo.

echo+++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
echo ADD MEG A YOUTUBE VIDE� LINKJ�T, AMINEK A HANGANYAG�T SZERETN�D LET�LTENI:
echo+++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
echo.
set /p channel_url="URL: "
echo+++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
echo.

"%yt_dlp_path%" -ciw -f bestaudio -x --audio-format wav --ffmpeg-location "%ffmpeg_path%" --concurrent-fragments 10 --http-chunk-size 10M --force-ipv4 --ppa "ffmpeg:-c:a pcm_s16le -threads 4" -o "%%(uploader)s_%%(title)s.%%(ext)s" "%channel_url%" || echo HIBA A LET�LT�S SOR�N!

echo ===================================================
echo             A TARTALOM LET�LT�SE SIKER�LT!
echo ===================================================
echo.

echo ===================================================
echo   SZERETN�L M�SIK HANGANYAGOT IS LET�LTENI? (Y/N)
echo ===================================================
echo.
set /p user_input="V�lasz: "
if /I "%user_input%"=="Y" goto start
if /I "%user_input%"=="N" exit
goto start
