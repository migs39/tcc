// Comando admin-seed: cria o primeiro administrador (RF01). Recusa se já
// existir algum; os seguintes são cadastrados pela área administrativa.
//
// Uso:
//
//	go run ./cmd/admin-seed -nome "Fulano" -email fulano@exemplo.com
//
// A senha é lida da variável ADMIN_SENHA ou, na falta, da primeira linha da
// entrada padrão, para não ficar no histórico do terminal.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"tcc/site/internal/admin"
	"tcc/site/internal/db"
)

const urlPadrao = "postgres://livraria:livraria_dev@localhost:5432/livraria?sslmode=disable"

func main() {
	nome := flag.String("nome", "", "nome do administrador")
	email := flag.String("email", "", "e-mail do administrador")
	flag.Parse()
	if *nome == "" || *email == "" {
		fmt.Fprintln(os.Stderr, "informe -nome e -email")
		os.Exit(2)
	}

	senha := os.Getenv("ADMIN_SENHA")
	if senha == "" {
		fmt.Fprint(os.Stderr, "Senha: ")
		linha, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && linha == "" {
			fmt.Fprintln(os.Stderr, "\nnão foi possível ler a senha")
			os.Exit(2)
		}
		senha = strings.TrimRight(linha, "\r\n")
	}

	if err := criar(*nome, *email, senha); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func criar(nome, email, senha string) error {
	ctx := context.Background()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = urlPadrao
	}
	pool, err := db.Conectar(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrar(ctx, pool); err != nil {
		return err
	}
	svc, err := admin.NovoServico(pool)
	if err != nil {
		return err
	}
	a, err := svc.CadastrarPrimeiro(ctx, nome, email, senha)
	if err != nil {
		return err
	}
	fmt.Printf("Administrador criado (id %d).\n", a.ID)
	return nil
}
