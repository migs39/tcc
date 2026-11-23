@echo off
rem Encerra o site, a CDN e a janela do programa local iniciados pelo
rem iniciar_demo.bat. O Postgres (Docker) e o tunel do Tailscale continuam;
rem para tirar o acesso pela internet, use fechar_acesso_publico.bat.
taskkill /f /im site.exe >nul 2>&1
taskkill /f /im cdn.exe >nul 2>&1
taskkill /f /im "Livraria OT.exe" >nul 2>&1
echo Livraria OT parada.
ping -n 3 127.0.0.1 >nul
