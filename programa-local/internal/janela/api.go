package janela

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"tcc/programa-local/internal/cliente"
	"tcc/protocolo"
)

var errWebView = errors.New("não foi possível abrir a janela: instale o Microsoft Edge WebView2 Runtime")

// API são as operações que a página pode pedir. Nenhuma recebe ou devolve a
// semente ou as chaves.
type API struct {
	c  *cliente.Cliente
	mu sync.Mutex // uma operação de compra/download por vez
}

// NovaAPI é usada nos testes e pela janela.
func NovaAPI(c *cliente.Cliente) *API { return &API{c: c} }

// Despachar chama o método pelo nome com os argumentos em JSON (lista).
func (a *API) Despachar(ctx context.Context, metodo string, args json.RawMessage) (any, error) {
	var lista []json.RawMessage
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &lista); err != nil {
			return nil, errors.New("argumentos inválidos")
		}
	}
	arg := func(i int, v any) error {
		if i >= len(lista) {
			return errors.New("argumentos faltando")
		}
		return json.Unmarshal(lista[i], v)
	}
	switch metodo {
	case "info":
		return a.Info(), nil
	case "catalogo":
		var sinc bool
		arg(0, &sinc)
		return a.Catalogo(ctx, sinc)
	case "comprar":
		var p PedidoJanela
		if err := arg(0, &p); err != nil {
			return nil, err
		}
		return a.Comprar(ctx, p)
	case "compras":
		return a.Compras()
	case "baixar":
		var id string
		if err := arg(0, &id); err != nil {
			return nil, err
		}
		return a.Baixar(ctx, id)
	case "recuperar":
		var conteudo string
		if err := arg(0, &conteudo); err != nil {
			return nil, err
		}
		return a.Recuperar(ctx, conteudo)
	case "reenviar":
		var id string
		if err := arg(0, &id); err != nil {
			return nil, err
		}
		return a.c.Reenviar(ctx, id)
	case "abrir":
		var caminho string
		if err := arg(0, &caminho); err != nil {
			return nil, err
		}
		return nil, a.Abrir(caminho)
	case "abrirPasta":
		os.MkdirAll(a.c.Biblioteca(), 0o700)
		return nil, abrirNoSistema(a.c.Biblioteca())
	}
	return nil, fmt.Errorf("método desconhecido: %s", metodo)
}

type Info struct {
	Site, CDN, Pasta string
}

func (a *API) Info() Info { return Info{a.c.SiteURL, a.c.CDNURL, a.c.Pasta} }

// ---------------------------------------------------------------- catálogo

type LivroJanela struct {
	ID        int64  `json:"id"`
	Titulo    string `json:"titulo"`
	Autor     string `json:"autor"`
	Categoria string `json:"categoria"`
	Descricao string `json:"descricao"`
	Ano       int    `json:"ano,omitempty"`
	Editora   string `json:"editora,omitempty"`
	FaixaID   int64  `json:"faixa_id"`
	Preco     int    `json:"preco"`
	Capa      string `json:"capa,omitempty"` // data URL da miniatura
}

type GrupoJanela struct {
	FaixaID   int64 `json:"faixa_id"`
	Preco     int   `json:"preco"`
	Versao    int   `json:"versao"`
	Livros    int   `json:"livros"`
	Compravel bool  `json:"compravel"`
}

type CatalogoJanela struct {
	Versao     int64         `json:"versao"`
	Offline    bool          `json:"offline"`
	Aviso      string        `json:"aviso,omitempty"`
	Categorias []string      `json:"categorias"`
	Grupos     []GrupoJanela `json:"grupos"`
	Livros     []LivroJanela `json:"livros"`
	Minimo     int           `json:"minimo"`
}

// Catalogo devolve o catálogo para a página. Com sinc, baixa o pacote inteiro
// antes (a mesma requisição de todos, D-007); sem rede, usa o guardado.
func (a *API) Catalogo(ctx context.Context, sinc bool) (*CatalogoJanela, error) {
	var pb *protocolo.PacoteBaixado
	var err error
	out := &CatalogoJanela{Minimo: protocolo.MinimoGrupo}
	if sinc {
		if pb, err = a.c.Sincronizar(ctx); err != nil {
			out.Offline, out.Aviso = true, "Não foi possível atualizar o catálogo: "+err.Error()
		}
	}
	if pb == nil {
		if pb, err = a.c.PacoteLocal(); err != nil {
			if out.Aviso != "" {
				return nil, errors.New(out.Aviso)
			}
			return nil, err
		}
	}
	// O ZIP local já veio conferido; as miniaturas são lidas dele.
	cat := pb.Catalogo
	out.Versao = pb.Versao
	nomes := map[int64]string{}
	for _, x := range cat.Categorias {
		nomes[x.ID] = x.Nome
		out.Categorias = append(out.Categorias, x.Nome)
	}
	sort.Strings(out.Categorias)
	for _, g := range cat.Grupos {
		out.Grupos = append(out.Grupos, GrupoJanela{g.FaixaID, g.PrecoCentavos, g.VersaoNumero, len(g.Livros),
			len(g.Livros) >= protocolo.MinimoGrupo && len(g.ChaveToken) > 0})
	}
	for _, l := range cat.Livros {
		lj := LivroJanela{ID: l.ID, Titulo: l.Titulo, Autor: l.Autor, Categoria: nomes[l.CategoriaID], Descricao: l.Descricao,
			Ano: l.Ano, Editora: l.Editora, FaixaID: l.FaixaID, Preco: l.PrecoCentavos}
		if l.Miniatura != "" && pb.ZIP != nil {
			if f, err := pb.ZIP.Open(l.Miniatura); err == nil {
				b, _ := io.ReadAll(io.LimitReader(f, 1<<20))
				f.Close()
				lj.Capa = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b)
			}
		}
		out.Livros = append(out.Livros, lj)
	}
	if out.Grupos == nil {
		out.Grupos = []GrupoJanela{}
	}
	if out.Livros == nil {
		out.Livros = []LivroJanela{}
	}
	return out, nil
}

// ---------------------------------------------------------------- compras

type PedidoJanela struct {
	Email     string  `json:"email"`
	Livros    []int64 `json:"livros"`
	Pagamento string  `json:"pagamento"`
	Adiar     bool    `json:"adiar"`
}

type CompraJanela struct {
	ID        string   `json:"id"`
	Email     string   `json:"email"`
	Titulos   []string `json:"titulos"`
	Livros    []int64  `json:"livros"`
	Baixados  []string `json:"baixados"`
	Estado    string   `json:"estado"` // "aguardando", "pendente", "baixada"
	Erro      string   `json:"erro,omitempty"`
	Pendentes int      `json:"pendentes"`
}

func resumo(cp *cliente.Compra) CompraJanela {
	cj := CompraJanela{ID: cp.CompraID, Email: cp.Email, Titulos: cp.Titulos, Livros: cp.Livros, Baixados: cp.Baixados}
	if cj.Baixados == nil {
		cj.Baixados = make([]string, len(cp.Livros))
	}
	switch {
	case cp.Resposta == nil:
		cj.Estado = "aguardando"
	default:
		cj.Estado = "baixada"
		for _, b := range cj.Baixados {
			if b == "" {
				cj.Estado = "pendente"
				cj.Pendentes++
			}
		}
	}
	return cj
}

// Comprar faz a compra e, sem Adiar, baixa e decifra em seguida.
func (a *API) Comprar(ctx context.Context, p PedidoJanela) (CompraJanela, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	pag := p.Pagamento
	if pag == "" {
		pag = "aprovar"
	}
	cp, err := a.c.Comprar(ctx, strings.TrimSpace(p.Email), p.Livros, pag)
	if err != nil {
		if cp != nil {
			cj := resumo(cp)
			cj.Erro = err.Error()
			return cj, nil
		}
		return CompraJanela{}, err
	}
	if !p.Adiar {
		if err := a.c.Baixar(ctx, cp); err != nil {
			cj := resumo(cp)
			cj.Erro = err.Error()
			return cj, nil
		}
	}
	return resumo(cp), nil
}

func (a *API) Compras() ([]CompraJanela, error) {
	cps, err := a.c.Compras()
	if err != nil {
		return nil, err
	}
	out := []CompraJanela{}
	for _, cp := range cps {
		out = append(out, resumo(cp))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Estado < out[j].Estado })
	return out, nil
}

func (a *API) achar(id string) (*cliente.Compra, error) {
	cps, err := a.c.Compras()
	if err != nil {
		return nil, err
	}
	for _, cp := range cps {
		if cp.CompraID == id && id != "" {
			return cp, nil
		}
	}
	return nil, errors.New("compra não encontrada neste computador")
}

func (a *API) Baixar(ctx context.Context, id string) (CompraJanela, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp, err := a.achar(id)
	if err != nil {
		return CompraJanela{}, err
	}
	err = a.c.Baixar(ctx, cp)
	cj := resumo(cp)
	if err != nil {
		cj.Erro = err.Error()
	}
	return cj, nil
}

// Recuperar recebe o conteúdo de resposta-ot.json (anexo do e-mail).
func (a *API) Recuperar(ctx context.Context, conteudo string) (CompraJanela, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var r protocolo.RespostaOT
	if err := json.Unmarshal([]byte(conteudo), &r); err != nil || r.Formato != protocolo.FormatoResposta {
		return CompraJanela{}, errors.New("o arquivo não é uma resposta do OT (resposta-ot.json do e-mail)")
	}
	cp, err := a.c.Recuperar(ctx, &r)
	if err != nil {
		return CompraJanela{}, err
	}
	err = a.c.Baixar(ctx, cp)
	cj := resumo(cp)
	if err != nil {
		cj.Erro = err.Error()
	}
	return cj, nil
}

// Abrir só abre arquivos da biblioteca.
func (a *API) Abrir(caminho string) error {
	bib, _ := filepath.Abs(a.c.Biblioteca())
	alvo, err := filepath.Abs(caminho)
	if err != nil || !strings.HasPrefix(alvo, bib+string(filepath.Separator)) {
		return errors.New("arquivo fora da biblioteca")
	}
	return abrirNoSistema(alvo)
}
