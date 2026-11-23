@echo off
set LOG=%~dp0build_libote.log
call "C:\Program Files (x86)\Microsoft Visual Studio\18\BuildTools\Common7\Tools\VsDevCmd.bat" -arch=x64 -no_logo
rem Só para este processo: o script de build do libsodium é PowerShell.
set PSExecutionPolicyPreference=Bypass
cd /d "%~dp0..\ot-helper\deps"
(
echo === inicio %date% %time%
if not exist libOTe git clone --depth 1 https://github.com/osu-crypto/libOTe.git
cd libOTe
git log -1 --format=%%H
python build.py --sodium -D ENABLE_MR=ON -D ENABLE_IKNP=OFF -D ENABLE_SILENTOT=OFF -D ENABLE_SOFTSPOKEN_OT=OFF -D ENABLE_KOS=OFF -D ENABLE_DELTA_KOS=OFF --install=%~dp0..\ot-helper\deps\install
echo === EXIT %errorlevel%
if exist "%~dp0..\ot-helper\deps\install\lib\cmake\libOTe" (echo === INSTALADO) else (echo === NAO INSTALADO)
echo === fim %date% %time%
) > "%LOG%" 2>&1
