@echo off
rem Expoe o site (443) e a CDN (8443) pela internet com o Tailscale Funnel.
rem O TLS termina nesta maquina: o Tailscale so repassa bytes cifrados e nao
rem ve compras nem downloads (premissa 5, nao conivencia site-CDN).
rem Antes: iniciar_demo.bat no ar e "tailscale login" feito uma vez.
rem Para fechar: fechar_acesso_publico.bat
set TS="C:\Program Files\Tailscale\tailscale.exe"
%TS% funnel --bg --https=443 http://localhost:8080
%TS% funnel --bg --https=8443 http://localhost:8081
%TS% funnel status
echo.
echo Para gerar o cliente apontando para estes enderecos:
echo   set PUBLICO=https://^<maquina^>.^<tailnet^>.ts.net
echo   build_app.bat
