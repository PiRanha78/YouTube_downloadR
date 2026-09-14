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
echo Ez  az  ingyenes  program  a  Youtube  platformr�l  ment  le vide�kat
echo az el�rhet� legjobb felbont�sban.
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
echo               YOUTUBE VIDE� LET�LT�SE
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

echo+++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
echo ADD MEG A YOUTUBE VIDE� LINKJ�T, AMIT SZERETN�L LET�LTENI:
echo+++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
echo.
set /p video_url="URL: "
echo+++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
echo.
"%yt_dlp_path%" --ffmpeg-location "%ffmpeg_path%" -f "bestvideo+bestaudio" --merge-output-format mp4 -o "%%(title)s [%%(id)s].mp4" "%video_url%"

if %errorlevel% neq 0 (
    echo HIBA A LET�LT�S SOR�N!
    pause
    exit /b
)

echo ===================================================
echo            A TARTALOM LET�LT�SE SIKER�LT!
echo ===================================================
echo.

echo ===================================================
echo      SZERETN�L M�SIK VIDE�T IS LET�LTENI? (Y/N)
echo ===================================================
echo.
set /p choice=
if /i "%choice%" == "Y" goto START
if /i "%choice%" == "N" exit

echo �rv�nytelen v�lasz, kil�p�s...
exit
