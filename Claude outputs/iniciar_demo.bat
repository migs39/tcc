@echo off
rem Sobe o Postgres, o site, a CDN e abre a janela do programa local.
rem Rode antes o build_app.bat. O Docker Desktop precisa estar aberto.
for %%I in ("%~dp0..") do set RAIZ=%%~fI
set /p CHAVE=<"%RAIZ%\bin\chave_pacote.pub"
set SEGREDO_CDN=demo-local
cd /d "%RAIZ%"
docker compose up -d --wait
start "Livraria OT - site" /d "%RAIZ%\site" cmd /k "set OT_HELPER=%RAIZ%\bin\ot-helper.exe&& %RAIZ%\bin\site.exe"
timeout /t 3 /nobreak >nul
start "Livraria OT - CDN" /d "%RAIZ%\cdn" cmd /k "set CHAVE_PACOTE_PUB=%CHAVE%&& %RAIZ%\bin\cdn.exe"
timeout /t 2 /nobreak >nul
start "" "%RAIZ%\bin\Livraria OT.exe"
start "" http://localhost:8080/
echo Site: http://localhost:8080  ^|  Admin: http://localhost:8080/admin/  ^|  CDN: http://localhost:8081/painel (senha: %SEGREDO_CDN%)
