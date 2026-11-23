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
rem Download do programa local: o site so aponta para o Release do GitHub (D-004)
rem e mostra o SHA-256 do zip, que precisa ser o mesmo arquivo publicado la.
set CLIENTE_URL=https://github.com/migs39/tcc/releases/latest/download/LivrariaOT-cliente.zip
if exist "%RAIZ%\bin\LivrariaOT-cliente.zip" for /f %%H in ('powershell -NoProfile -Command "(Get-FileHash '%RAIZ%\bin\LivrariaOT-cliente.zip').Hash"') do set CLIENTE_SHA256=%%H
cd /d "%RAIZ%"
docker compose up -d --wait
rem Site e CDN rodam em segundo plano, sem janela; o que escrevem vai para
rem bin\logs\. Uma execucao anterior e encerrada antes (portas 8080 e 8081).
taskkill /f /im site.exe >nul 2>&1
taskkill /f /im cdn.exe >nul 2>&1
if not exist "%RAIZ%\bin\logs" mkdir "%RAIZ%\bin\logs"
set OT_HELPER=%RAIZ%\bin\ot-helper.exe
set CHAVE_PACOTE_PUB=%CHAVE%
powershell -NoProfile -Command "Start-Process -FilePath '%RAIZ%\bin\site.exe' -WorkingDirectory '%RAIZ%\site' -WindowStyle Hidden -RedirectStandardOutput '%RAIZ%\bin\logs\site.out.log' -RedirectStandardError '%RAIZ%\bin\logs\site.log'"
timeout /t 3 /nobreak >nul
powershell -NoProfile -Command "Start-Process -FilePath '%RAIZ%\bin\cdn.exe' -WorkingDirectory '%RAIZ%\cdn' -WindowStyle Hidden -RedirectStandardOutput '%RAIZ%\bin\logs\cdn.out.log' -RedirectStandardError '%RAIZ%\bin\logs\cdn.log'"
timeout /t 2 /nobreak >nul
start "" "%RAIZ%\bin\Livraria OT.exe"
start "" http://localhost:8080/
rem Sem pause: a janela deste script fecha sozinha. Para parar: parar_demo.bat.
