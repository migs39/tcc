// Comando livraria-janela: a interface gráfica do programa local (D-014),
// para Windows. Usa as mesmas variáveis de ambiente do comando livraria e a
// chave pública do catálogo embutida na compilação:
//
//	go build -ldflags "-H=windowsgui -X main.chavePacote=<hex>" -o "Livraria OT.exe" ./cmd/livraria-janela
//
// O ot-helper.exe fica ao lado do executável.
package main

import (
	"os"

	"tcc/programa-local/internal/cliente"
	"tcc/programa-local/internal/janela"
)

var chavePacote = ""

func main() {
	c, err := cliente.Configurar(chavePacote)
	if err == nil {
		err = janela.Abrir(c)
	}
	if err != nil {
		mostrarErro(err.Error())
		os.Exit(1)
	}
}
