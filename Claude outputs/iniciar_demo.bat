@echo off
rem Sobe o Postgres, o site, a CDN e abre a janela do programa local.
rem Rode antes o build_app.bat. O Docker Desktop precisa estar aberto.
for %%I in ("%~dp0..") do set RAIZ=%%~fI
set /p CHAVE=<"%RAIZ%\bin\chave_pacote.pub"
rem Segredo entre site e CDN: aleatorio, criado uma vez e guardado em bin
rem (fora do git). Protege a publicacao de livros na CDN, o reporte e o /painel.
if not exist "%RAIZ%\bin\segredo_cdn.txt" powershell -NoProfile -Command "$b=New-Object byte[] 24; [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); [Convert]::ToBase64String($b).Replace('+','-').Replace('/','_') | Set-Content -NoNewline -Encoding ascii '%RAIZ%\bin\segredo_cdn.txt'"
set /p SEGREDO_CDN=<"%RAIZ%\bin\segredo_cdn.txt"
rem E-mail real (RF14): bin\smtp.env (modelo em smtp.env.exemplo), fora do git.
rem Sem esse arquivo, os e-mails vao para site\caixa_saida.
if exist "%RAIZ%\bin\smtp.env" for /f "usebackq tokens=1,* delims==" %%A in (`findstr /b "SMTP_" "%RAIZ%\bin\smtp.env"`) do set "%%A=%%B"
cd /d "%RAIZ%"
docker compose up -d --wait
start "Livraria OT - site" /d "%RAIZ%\site" cmd /k "set OT_HELPER=%RAIZ%\bin\ot-helper.exe&& %RAIZ%\bin\site.exe"
timeout /t 3 /nobreak >nul
start "Livraria OT - CDN" /d "%RAIZ%\cdn" cmd /k "set CHAVE_PACOTE_PUB=%CHAVE%&& %RAIZ%\bin\cdn.exe"
timeout /t 2 /nobreak >nul
start "" "%RAIZ%\bin\Livraria OT.exe"
start "" http://localhost:8080/
echo Site: http://localhost:8080  ^|  Admin: http://localhost:8080/admin/  ^|  CDN: http://localhost:8081/painel (usuario qualquer, senha em bin\segredo_cdn.txt)
echo Acesso pela internet: rode abrir_acesso_publico.bat
