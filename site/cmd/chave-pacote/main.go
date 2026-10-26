// Comando chave-pacote: mostra a chave pública que assina o pacote do
// catálogo (D-006), para embutir no programa local. Cria a chave se o arquivo
// não existir.
//
// Uso:
//
//	go run ./cmd/chave-pacote [-arquivo chave_pacote.ed25519]
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"tcc/site/internal/pacote"
)

func main() {
	padrao := os.Getenv("CHAVE_PACOTE")
	if padrao == "" {
		padrao = "chave_pacote.ed25519"
	}
	arquivo := flag.String("arquivo", padrao, "arquivo da chave privada (semente em hex)")
	flag.Parse()

	chave, criada, err := pacote.CarregarOuCriarChave(*arquivo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
	if criada {
		fmt.Fprintln(os.Stderr, "chave nova criada em", *arquivo)
	}
	fmt.Println(hex.EncodeToString(chave.Public().(ed25519.PublicKey)))
}
