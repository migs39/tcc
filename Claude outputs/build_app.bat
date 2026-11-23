@echo off
setlocal EnableDelayedExpansion
set LOG=%~dp0build_app.log
set RAIZ=%~dp0..
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
go build -o "%RAIZ%\bin\site.exe" ./cmd/site
go build -o "%RAIZ%\bin\admin-seed.exe" ./cmd/admin-seed
cd /d "%RAIZ%\cdn"
go build -o "%RAIZ%\bin\cdn.exe" ./cmd/cdn
cd /d "%RAIZ%\programa-local"
go build -ldflags "-X main.chavePacote=!CHAVE!" -o "%RAIZ%\bin\livraria.exe" ./cmd/livraria
go build -ldflags "-H=windowsgui -X main.chavePacote=!CHAVE!" -o "%RAIZ%\bin\Livraria OT.exe" ./cmd/livraria-janela
echo !CHAVE!> "%RAIZ%\bin\chave_pacote.pub"
dir /b "%RAIZ%\bin"
echo === testes
set OT_HELPER=%RAIZ%\bin\ot-helper.exe
for %%M in (protocolo site cdn programa-local) do (
  cd /d "%RAIZ%\%%M"
  go test -count=1 ./...
)
echo === fim
) > "%LOG%" 2>&1
