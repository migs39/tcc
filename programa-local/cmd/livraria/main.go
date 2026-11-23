// Comando livraria: o programa local do cliente (RF04, RF10–RF12, D-004,
// D-006, D-012). Fala só com o site e com a CDN.
//
// Uso:
//
//	livraria catalogo                       baixa o catálogo inteiro
//	livraria buscar [filtros]               busca local (-texto -titulo -autor -categoria -preco)
//	livraria grupos                         grupos de mesmo preço
//	livraria comprar -email E -livros 3,7   compra privada (um único grupo)
//	        [-pagamento aprovar] [-adiar]   (-adiar: não baixa agora; use `baixar` depois)
//	livraria baixar [compra]                baixa e decifra os livros pendentes
//	livraria recuperar resposta-ot.json     conclui uma compra interrompida com o e-mail
//	livraria reenviar compra                pede o reenvio da resposta do OT (RF15)
//	livraria compras                        compras guardadas neste computador
//
// Variáveis de ambiente:
//
//	LIVRARIA_DADOS    pasta de dados (padrão: pasta de configuração do usuário/livraria-ot)
//	SITE_URL          padrão http://localhost:8080
//	CDN_URL           padrão http://localhost:8081
//	OT_HELPER         caminho do ot-helper (padrão: ao lado deste executável ou no PATH)
//	CHAVE_PACOTE_PUB  chave pública do pacote (hex), se não embutida na compilação
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"tcc/programa-local/internal/cliente"
	"tcc/protocolo"
)

// chavePacote é embutida na compilação:
//
//	go build -ldflags "-X main.chavePacote=<hex>" ./cmd/livraria
var chavePacote = ""

func main() {
	if err := rodar(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func novoCliente() (*cliente.Cliente, error) { return cliente.Configurar(chavePacote) }

func rodar(args []string) error {
	if len(args) == 0 {
		fmt.Println(`comandos: catalogo, buscar, grupos, comprar, baixar, recuperar, reenviar, compras`)
		return nil
	}
	c, err := novoCliente()
	if err != nil {
		return err
	}
	ctx := context.Background()
	cmd, resto := args[0], args[1:]
	switch cmd {
	case "catalogo":
		pb, err := c.Sincronizar(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("catálogo versão %d: %d livros, %d grupos (assinatura conferida)\n", pb.Versao, len(pb.Catalogo.Livros), len(pb.Catalogo.Grupos))
	case "buscar":
		return buscar(c, resto)
	case "grupos":
		cat, _, err := c.Catalogo()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "PREÇO\tLIVROS\tVERSÃO\tCOMPRÁVEL")
		for _, g := range cat.Grupos {
			ok := "sim"
			if len(g.Livros) < protocolo.MinimoGrupo {
				ok = fmt.Sprintf("não (menos de %d livros)", protocolo.MinimoGrupo)
			}
			fmt.Fprintf(w, "%s\t%d\t%d\t%s\n", cliente.Preco(g.PrecoCentavos), len(g.Livros), g.VersaoNumero, ok)
		}
		w.Flush()
	case "comprar":
		fs := flag.NewFlagSet("comprar", flag.ContinueOnError)
		email := fs.String("email", "", "e-mail para receber a resposta do OT")
		livros := fs.String("livros", "", "IDs dos livros, separados por vírgula")
		pag := fs.String("pagamento", "aprovar", "dados de pagamento (gateway mock: aprovar/recusar)")
		adiar := fs.Bool("adiar", false, "não baixar agora")
		if err := fs.Parse(resto); err != nil {
			return err
		}
		var ids []int64
		for _, s := range strings.Split(*livros, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if err != nil {
				return fmt.Errorf("livro inválido: %q", s)
			}
			ids = append(ids, id)
		}
		cp, err := c.Comprar(ctx, *email, ids, *pag)
		if err != nil {
			return err
		}
		fmt.Printf("compra %s aprovada: %d livro(s). A resposta do OT também foi para %s.\n", cp.CompraID, len(cp.Livros), cp.Email)
		if *adiar {
			fmt.Println("download adiado; rode `livraria baixar` quando quiser.")
			return nil
		}
		return baixar(ctx, c, cp)
	case "baixar":
		cps, err := c.Compras()
		if err != nil {
			return err
		}
		for _, cp := range cps {
			if len(resto) > 0 && cp.CompraID != resto[0] {
				continue
			}
			if cp.Resposta == nil || completa(cp) {
				continue
			}
			if err := baixar(ctx, c, cp); err != nil {
				return err
			}
		}
	case "recuperar":
		if len(resto) != 1 {
			return errors.New("uso: livraria recuperar <resposta-ot.json>")
		}
		b, err := os.ReadFile(resto[0])
		if err != nil {
			return err
		}
		var r protocolo.RespostaOT
		if err := json.Unmarshal(b, &r); err != nil {
			return fmt.Errorf("arquivo não é uma resposta do OT: %w", err)
		}
		cp, err := c.Recuperar(ctx, &r)
		if err != nil {
			return err
		}
		fmt.Printf("compra %s recuperada com a semente local.\n", cp.CompraID)
		return baixar(ctx, c, cp)
	case "reenviar":
		if len(resto) != 1 {
			return errors.New("uso: livraria reenviar <compra>")
		}
		m, err := c.Reenviar(ctx, resto[0])
		if err != nil {
			return err
		}
		fmt.Println(m)
	case "compras":
		cps, err := c.Compras()
		if err != nil {
			return err
		}
		sort.Slice(cps, func(i, j int) bool { return cps[i].CompraID < cps[j].CompraID })
		for _, cp := range cps {
			estado := "aguardando resposta (use `recuperar` com o e-mail)"
			if cp.Resposta != nil {
				estado = "pendente de download"
				if completa(cp) {
					estado = "baixada"
				}
			}
			id := cp.CompraID
			if id == "" {
				id = "(sem id)"
			}
			fmt.Printf("%s  %s  %s\n", id, strings.Join(cp.Titulos, "; "), estado)
		}
	default:
		return fmt.Errorf("comando desconhecido: %s", cmd)
	}
	return nil
}

func completa(cp *cliente.Compra) bool {
	if len(cp.Baixados) != len(cp.Livros) {
		return false
	}
	for _, b := range cp.Baixados {
		if b == "" {
			return false
		}
	}
	return true
}

func baixar(ctx context.Context, c *cliente.Cliente, cp *cliente.Compra) error {
	if err := c.Baixar(ctx, cp); err != nil {
		return err
	}
	for _, arq := range cp.Baixados {
		fmt.Println("decifrado:", arq)
	}
	return nil
}

func buscar(c *cliente.Cliente, args []string) error {
	fs := flag.NewFlagSet("buscar", flag.ContinueOnError)
	var f cliente.Filtro
	fs.StringVar(&f.Texto, "texto", "", "título, autor ou descrição")
	fs.StringVar(&f.Titulo, "titulo", "", "")
	fs.StringVar(&f.Autor, "autor", "", "")
	fs.StringVar(&f.Categoria, "categoria", "", "")
	precoTxt := fs.String("preco", "", "preço, ex.: 29,90")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *precoTxt != "" {
		v, err := strconv.ParseFloat(strings.ReplaceAll(*precoTxt, ",", "."), 64)
		if err != nil {
			return fmt.Errorf("preço inválido: %s", *precoTxt)
		}
		f.PrecoCentavos = int(v*100 + 0.5)
	}
	cat, _, err := c.Catalogo()
	if err != nil {
		return err
	}
	if len(fs.Args()) > 0 && f.Texto == "" {
		f.Texto = strings.Join(fs.Args(), " ")
	}
	cats := map[int64]string{}
	for _, x := range cat.Categorias {
		cats[x.ID] = x.Nome
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTÍTULO\tAUTOR\tCATEGORIA\tPREÇO")
	for _, l := range cliente.Buscar(cat, f) {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", l.ID, l.Titulo, l.Autor, cats[l.CategoriaID], cliente.Preco(l.PrecoCentavos))
	}
	return w.Flush()
}
