@echo off
rem Compila tudo em bin\ e roda os testes; a saida vai para build_app.log.
rem Opcional: PUBLICO=https://<maquina>.<tailnet>.ts.net embute no programa
rem local o site (porta 443) e a CDN (porta 8443) publicos, e gera
rem bin\LivrariaOT-cliente.zip para distribuir (fora do site, D-004).
setlocal EnableDelayedExpansion
set LOG=%~dp0build_app.log
set RAIZ=%~dp0..
set CRT=C:\Program Files (x86)\Microsoft Visual Studio\18\BuildTools\VC\Redist\MSVC\14.52.37025\x64\Microsoft.VC145.CRT
(
echo === inicio
for %%M in (protocolo site cdn programa-local) do (
  echo === tidy %%M
  cd /d "%RAIZ%\%%M"
  go mod tidy
  go vet ./...
)
if not exist "%RAIZ%\bin" mkdir "%RAIZ%\bin"
copy /y "%RAIZ%\ot-helper\build\ot-helper.exe" "%RAIZ%\bin\ot-helper.exe"
cd /d "%RAIZ%\site"
for /f "delims=" %%K in ('go run ./cmd/chave-pacote') do set CHAVE=%%K
echo chave publica: !CHAVE!
if "!CHAVE!"=="" (echo ERRO: chave vazia & exit /b 1)
rem -s -w: sem tabela de simbolos e depuracao (zip menor); -trimpath: sem caminhos desta maquina no binario.
set FLAGS=-s -w -X main.chavePacote=!CHAVE!
if not "%PUBLICO%"=="" (
  set FLAGS=!FLAGS! -X main.siteURL=%PUBLICO% -X main.cdnURL=%PUBLICO%:8443
  echo enderecos embutidos: site %PUBLICO%  cdn %PUBLICO%:8443
)
go build -o "%RAIZ%\bin\site.exe" ./cmd/site
go build -o "%RAIZ%\bin\admin-seed.exe" ./cmd/admin-seed
cd /d "%RAIZ%\cdn"
go build -o "%RAIZ%\bin\cdn.exe" ./cmd/cdn
cd /d "%RAIZ%\programa-local"
go build -trimpath -ldflags "!FLAGS!" -o "%RAIZ%\bin\livraria.exe" ./cmd/livraria
go build -trimpath -ldflags "-H=windowsgui !FLAGS!" -o "%RAIZ%\bin\Livraria OT.exe" ./cmd/livraria-janela
echo !CHAVE!> "%RAIZ%\bin\chave_pacote.pub"
if not "%PUBLICO%"=="" (
  echo === pacote do cliente
  set PAC=%RAIZ%\bin\cliente
  if exist "!PAC!" rmdir /s /q "!PAC!"
  mkdir "!PAC!"
  copy /y "%RAIZ%\bin\Livraria OT.exe" "!PAC!\"
  copy /y "%RAIZ%\bin\livraria.exe" "!PAC!\"
  copy /y "%RAIZ%\bin\ot-helper.exe" "!PAC!\"
  rem Runtime do Visual C++ ao lado do ot-helper (redistribuicao local).
  for %%D in (msvcp140.dll msvcp140_atomic_wait.dll vcruntime140.dll vcruntime140_1.dll) do copy /y "%CRT%\%%D" "!PAC!\"
  copy /y "%~dp0LEIA-ME-cliente.txt" "!PAC!\LEIA-ME.txt"
  powershell -NoProfile -Command "Compress-Archive -Force -Path '!PAC!\*' -DestinationPath '%RAIZ%\bin\LivrariaOT-cliente.zip'"
  powershell -NoProfile -Command "(Get-FileHash '%RAIZ%\bin\LivrariaOT-cliente.zip').Hash"
)
dir /b "%RAIZ%\bin"
echo === testes
set OT_HELPER=%RAIZ%\bin\ot-helper.exe
for %%M in (protocolo site cdn programa-local) do (
  cd /d "%RAIZ%\%%M"
  go test -count=1 ./...
)
echo === fim
) > "%LOG%" 2>&1
