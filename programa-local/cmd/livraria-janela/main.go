// Comando livraria-janela: a interface gráfica do programa local (D-014),
// para Windows. Usa as mesmas variáveis de ambiente do comando livraria e a
// chave pública do catálogo e os endereços embutidos na compilação:
//
//	go build -ldflags "-H=windowsgui -X main.chavePacote=<hex> -X main.siteURL=https://... -X main.cdnURL=https://..." -o "Livraria OT.exe" ./cmd/livraria-janela
//
// O ot-helper.exe fica ao lado do executável.
package main

import (
	"os"

	"tcc/programa-local/internal/cliente"
	"tcc/programa-local/internal/janela"
)

var chavePacote, siteURL, cdnURL string

func main() {
	c, err := cliente.Configurar(cliente.Embutidos{ChavePacote: chavePacote, SiteURL: siteURL, CDNURL: cdnURL})
	if err == nil {
		err = janela.Abrir(c)
	}
	if err != nil {
		mostrarErro(err.Error())
		os.Exit(1)
	}
}
