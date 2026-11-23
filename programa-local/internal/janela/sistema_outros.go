//go:build !windows

package janela

import (
	"errors"
	"os/exec"

	"tcc/programa-local/internal/cliente"
)

// Abrir: a janela gráfica existe só no Windows (WebView2). Nos outros
// sistemas, use a linha de comando.
func Abrir(*cliente.Cliente) error {
	return errors.New("a janela gráfica só está disponível no Windows; use os comandos de `livraria`")
}

func abrirNoSistema(caminho string) error { return exec.Command("xdg-open", caminho).Start() }
