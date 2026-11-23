@echo off
rem Tira o site e a CDN da internet (desfaz o abrir_acesso_publico.bat).
"C:\Program Files\Tailscale\tailscale.exe" funnel reset
"C:\Program Files\Tailscale\tailscale.exe" funnel status
