// Package cliente é o programa local (RF04, RF10–RF12, D-001, D-004, D-006):
// navega no catálogo baixado inteiro, gera a requisição de OT a partir de uma
// semente guardada só no computador do cliente, cega os tokens, fala com o
// site e com a CDN (e com mais ninguém), extrai as chaves e decifra os livros.
//
// A semente e as chaves nunca saem desta máquina. Perder a pasta de dados é
// perder o acesso aos livros não baixados (invariante 6).
package cliente

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"tcc/protocolo"
)

// Cliente guarda a configuração do programa.
type Cliente struct {
	Pasta       string // dados locais: catálogo, compras (com a semente), biblioteca
	SiteURL     string
	CDNURL      string
	ChavePacote ed25519.PublicKey
	OT          protocolo.OTHelper
	HTTP        *http.Client
}

func (c *Cliente) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

func (c *Cliente) caminho(partes ...string) string {
	return filepath.Join(append([]string{c.Pasta}, partes...)...)
}

// gravar grava um arquivo só legível pelo usuário (a semente está nele).
func gravar(caminho string, dados []byte) error {
	if err := os.MkdirAll(filepath.Dir(caminho), 0o700); err != nil {
		return err
	}
	tmp := caminho + ".tmp"
	if err := os.WriteFile(tmp, dados, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, caminho)
}

// ---------------------------------------------------------------- catálogo

type metaCatalogo struct {
	Versao     int64  `json:"versao"`
	Assinatura []byte `json:"assinatura"`
}

// Sincronizar baixa o pacote inteiro do catálogo (a mesma requisição de
// todos os clientes, D-007), confere a assinatura e recusa versão mais antiga
// que a já guardada.
func (c *Cliente) Sincronizar(ctx context.Context) (*protocolo.PacoteBaixado, error) {
	var meta metaCatalogo
	if b, err := os.ReadFile(c.caminho("catalogo.json")); err == nil {
		json.Unmarshal(b, &meta)
	}
	pb, err := protocolo.BaixarPacote(ctx, c.http(), c.SiteURL, c.ChavePacote, meta.Versao)
	if err != nil {
		return nil, err
	}
	if err := gravar(c.caminho("catalogo.zip"), pb.Dados); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(metaCatalogo{Versao: pb.Versao, Assinatura: pb.Assinatura})
	return pb, gravar(c.caminho("catalogo.json"), b)
}

// Catalogo abre o catálogo guardado (e confere de novo a assinatura).
func (c *Cliente) Catalogo() (protocolo.Catalogo, int64, error) {
	var meta metaCatalogo
	b, err := os.ReadFile(c.caminho("catalogo.json"))
	if err != nil {
		return protocolo.Catalogo{}, 0, errors.New("catálogo ainda não baixado; rode `livraria catalogo`")
	}
	json.Unmarshal(b, &meta)
	dados, err := os.ReadFile(c.caminho("catalogo.zip"))
	if err != nil {
		return protocolo.Catalogo{}, 0, err
	}
	// Confere de novo: o arquivo local pode ter sido trocado.
	cat, _, err := protocolo.AbrirPacote(c.ChavePacote, meta.Versao, meta.Assinatura, dados)
	return cat, meta.Versao, err
}

// PacoteLocal abre o pacote guardado, conferindo a assinatura, com o ZIP
// (miniaturas).
func (c *Cliente) PacoteLocal() (*protocolo.PacoteBaixado, error) {
	var meta metaCatalogo
	b, err := os.ReadFile(c.caminho("catalogo.json"))
	if err != nil {
		return nil, errors.New("catálogo ainda não baixado")
	}
	json.Unmarshal(b, &meta)
	dados, err := os.ReadFile(c.caminho("catalogo.zip"))
	if err != nil {
		return nil, err
	}
	cat, zr, err := protocolo.AbrirPacote(c.ChavePacote, meta.Versao, meta.Assinatura, dados)
	if err != nil {
		return nil, err
	}
	return &protocolo.PacoteBaixado{Versao: meta.Versao, Assinatura: meta.Assinatura, Dados: dados, Catalogo: cat, ZIP: zr}, nil
}

// Filtro de busca (RF04): tudo local, nada vai para a rede.
type Filtro struct {
	Texto, Titulo, Autor, Categoria string
	PrecoCentavos                   int
}

func contem(s, sub string) bool { return strings.Contains(strings.ToLower(s), strings.ToLower(sub)) }

// Buscar filtra os livros do catálogo guardado.
func Buscar(cat protocolo.Catalogo, f Filtro) []protocolo.Livro {
	cats := map[int64]string{}
	for _, x := range cat.Categorias {
		cats[x.ID] = x.Nome
	}
	var out []protocolo.Livro
	for _, l := range cat.Livros {
		if f.Titulo != "" && !contem(l.Titulo, f.Titulo) ||
			f.Autor != "" && !contem(l.Autor, f.Autor) ||
			f.Categoria != "" && !contem(cats[l.CategoriaID], f.Categoria) ||
			f.PrecoCentavos > 0 && l.PrecoCentavos != f.PrecoCentavos ||
			f.Texto != "" && !(contem(l.Titulo, f.Texto) || contem(l.Autor, f.Texto) || contem(l.Descricao, f.Texto)) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// ---------------------------------------------------------------- compra

// Compra é o estado local de uma compra. Contém a semente: é o segredo do
// cliente (RF11) e fica só neste computador.
type Compra struct {
	Formato       int                   `json:"formato"`
	Semente       string                `json:"semente"` // hex
	Email         string                `json:"email"`
	VersaoGrupoID int64                 `json:"versao_grupo_id"`
	FaixaID       int64                 `json:"faixa_id"`
	TamanhoGrupo  int                   `json:"tamanho_grupo"`
	Indices       []int                 `json:"indices"`
	Livros        []int64               `json:"livros"`
	Titulos       []string              `json:"titulos"`
	ChaveToken    []byte                `json:"chave_token"`
	CompraID      string                `json:"compra_id,omitempty"`
	Resposta      *protocolo.RespostaOT `json:"resposta,omitempty"`
	Baixados      []string              `json:"baixados,omitempty"` // arquivo decifrado de cada livro
}

func (cp *Compra) semente() (protocolo.Semente, error) {
	var s protocolo.Semente
	b, err := hex.DecodeString(cp.Semente)
	if err != nil || len(b) != len(s) {
		return s, errors.New("semente corrompida")
	}
	copy(s[:], b)
	return s, nil
}

func (c *Cliente) arquivoCompra(cp *Compra) string {
	nome := cp.CompraID
	if nome == "" {
		nome = "pendente-" + cp.Semente[:16]
	}
	return c.caminho("compras", nome+".json")
}

func (c *Cliente) salvar(cp *Compra) error {
	b, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	return gravar(c.arquivoCompra(cp), b)
}

// Compras lista as compras guardadas.
func (c *Cliente) Compras() ([]*Compra, error) {
	arqs, _ := filepath.Glob(c.caminho("compras", "*.json"))
	var out []*Compra
	for _, a := range arqs {
		b, err := os.ReadFile(a)
		if err != nil {
			return nil, err
		}
		var cp Compra
		if err := json.Unmarshal(b, &cp); err != nil {
			return nil, fmt.Errorf("%s: %w", a, err)
		}
		out = append(out, &cp)
	}
	return out, nil
}

// Comprar faz os passos 1–4 do fluxo para livros de um mesmo grupo. A semente
// e as escolhas são gravadas antes do envio (D-001). Pagamento vai para o
// gateway (mock: "aprovar" ou "recusar").
func (c *Cliente) Comprar(ctx context.Context, email string, livros []int64, pagamento string) (*Compra, error) {
	if len(livros) == 0 || len(livros) > protocolo.MaximoPorCompra {
		return nil, fmt.Errorf("escolha entre 1 e %d livros", protocolo.MaximoPorCompra)
	}
	// Sempre o catálogo atual: a compra referencia a versão vigente do grupo.
	pb, err := c.Sincronizar(ctx)
	if err != nil {
		return nil, err
	}
	cat := pb.Catalogo
	porID := map[int64]protocolo.Livro{}
	for _, l := range cat.Livros {
		porID[l.ID] = l
	}
	primeiro, ok := porID[livros[0]]
	if !ok {
		return nil, fmt.Errorf("livro %d não está no catálogo", livros[0])
	}
	var g protocolo.Grupo
	for _, x := range cat.Grupos {
		if x.FaixaID == primeiro.FaixaID {
			g = x
		}
	}
	if g.VersaoGrupoID == 0 || len(g.ChaveToken) == 0 {
		return nil, errors.New("grupo indisponível para compra")
	}
	if len(g.Livros) < protocolo.MinimoGrupo {
		return nil, fmt.Errorf("o grupo de %s tem menos de %d livros; a compra não esconderia a escolha", preco(g.PrecoCentavos), protocolo.MinimoGrupo)
	}
	pos := map[int64]int{}
	for i, id := range g.Livros {
		pos[id] = i
	}
	cp := &Compra{Formato: 1, Email: email, VersaoGrupoID: g.VersaoGrupoID, FaixaID: g.FaixaID,
		TamanhoGrupo: len(g.Livros), ChaveToken: g.ChaveToken}
	for _, id := range livros {
		i, ok := pos[id]
		if !ok {
			return nil, fmt.Errorf("livro %d não é do grupo de %s; uma compra é sempre de um único grupo de mesmo preço", id, preco(g.PrecoCentavos))
		}
		cp.Indices = append(cp.Indices, i)
		cp.Livros = append(cp.Livros, id)
		cp.Titulos = append(cp.Titulos, porID[id].Titulo)
	}

	sem, err := protocolo.NovaSemente()
	if err != nil {
		return nil, err
	}
	cp.Semente = hex.EncodeToString(sem[:])
	pedido, err := c.montarPedido(ctx, cp, sem)
	if err != nil {
		return nil, err
	}
	pedido.Pagamento = pagamento
	// D-001: semente e escolhas no disco antes de qualquer envio.
	if err := c.salvar(cp); err != nil {
		return nil, err
	}

	corpo, _ := json.Marshal(pedido)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.SiteURL, "/")+"/api/compras", bytes.NewReader(corpo))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return cp, fmt.Errorf("falha ao falar com o site (a compra pode ter sido feita; se receber o e-mail, use `livraria recuperar`): %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	var res protocolo.ResultadoCompra
	if err := json.Unmarshal(b, &res); err != nil || (res.Status == "" && resp.StatusCode != http.StatusOK) {
		var e struct{ Erro string }
		json.Unmarshal(b, &e)
		os.Remove(c.arquivoCompra(cp))
		return nil, fmt.Errorf("site recusou a compra (%d): %s", resp.StatusCode, e.Erro)
	}
	if res.Status != "aprovada" || res.Resposta == nil {
		os.Remove(c.arquivoCompra(cp))
		return nil, fmt.Errorf("compra não aprovada: %s", res.Mensagem)
	}
	return cp, c.registrarResposta(ctx, cp, res.Resposta)
}

func (c *Cliente) montarPedido(ctx context.Context, cp *Compra, sem protocolo.Semente) (protocolo.PedidoCompra, error) {
	esc, err := protocolo.EscolhasOT(cp.Indices, cp.TamanhoGrupo)
	if err != nil {
		return protocolo.PedidoCompra{}, err
	}
	req, err := c.OT.Requisicao(ctx, sem.SementeOT(), esc)
	if err != nil {
		return protocolo.PedidoCompra{}, err
	}
	pk, err := protocolo.LerPublica(cp.ChaveToken)
	if err != nil {
		return protocolo.PedidoCompra{}, err
	}
	tp, err := protocolo.CegarTokens(pk, sem.Fluxo("tokens"), len(cp.Indices))
	if err != nil {
		return protocolo.PedidoCompra{}, err
	}
	return protocolo.PedidoCompra{Email: cp.Email, VersaoGrupoID: cp.VersaoGrupoID, Quantidade: len(cp.Indices),
		RequisicaoOT: req, Cegados: tp.Cegados}, nil
}

// tokens refaz os tokens a partir da semente e das assinaturas cegas; falha
// se a resposta não for desta compra.
func (c *Cliente) tokens(cp *Compra, r *protocolo.RespostaOT) ([]protocolo.Token, error) {
	sem, err := cp.semente()
	if err != nil {
		return nil, err
	}
	pk, err := protocolo.LerPublica(cp.ChaveToken)
	if err != nil {
		return nil, err
	}
	tp, err := protocolo.CegarTokens(pk, sem.Fluxo("tokens"), len(cp.Indices))
	if err != nil {
		return nil, err
	}
	return tp.Finalizar(r.AssinaturasCegas)
}

// chaves extrai as chaves dos livros escolhidos (passo 7). Ficam só em
// memória.
func (c *Cliente) chaves(ctx context.Context, cp *Compra) ([][protocolo.TamChave]byte, error) {
	sem, err := cp.semente()
	if err != nil {
		return nil, err
	}
	esc, err := protocolo.EscolhasOT(cp.Indices, cp.TamanhoGrupo)
	if err != nil {
		return nil, err
	}
	rec, err := c.OT.Extrair(ctx, sem.SementeOT(), esc, cp.Resposta.RespostaSender)
	if err != nil {
		return nil, err
	}
	return protocolo.AbrirGrupo(cp.Resposta.Cifras, rec, cp.Indices, cp.TamanhoGrupo)
}

func (c *Cliente) registrarResposta(ctx context.Context, cp *Compra, r *protocolo.RespostaOT) error {
	if r.VersaoGrupoID != cp.VersaoGrupoID || r.Quantidade != len(cp.Indices) {
		return errors.New("a resposta não é desta compra")
	}
	if _, err := c.tokens(cp, r); err != nil {
		return errors.New("a resposta não é desta compra (tokens não conferem)")
	}
	antigo := c.arquivoCompra(cp)
	cp.CompraID, cp.Resposta = r.CompraID, r
	if err := c.salvar(cp); err != nil {
		return err
	}
	if antigo != c.arquivoCompra(cp) {
		os.Remove(antigo)
	}
	return nil
}

// Recuperar usa a resposta do OT recebida por e-mail para concluir uma compra
// interrompida (D-001, RF14): procura, entre as compras pendentes, a que
// confere com a resposta.
func (c *Cliente) Recuperar(ctx context.Context, r *protocolo.RespostaOT) (*Compra, error) {
	todas, err := c.Compras()
	if err != nil {
		return nil, err
	}
	for _, cp := range todas {
		if cp.CompraID == r.CompraID && cp.Resposta != nil {
			return cp, nil // já registrada
		}
	}
	for _, cp := range todas {
		if cp.Resposta != nil || cp.VersaoGrupoID != r.VersaoGrupoID || len(cp.Indices) != r.Quantidade {
			continue
		}
		if c.registrarResposta(ctx, cp, r) == nil {
			return cp, nil
		}
	}
	return nil, errors.New("nenhuma compra guardada neste computador confere com esta resposta; sem a semente local não há como extrair as chaves")
}

var naoNome = regexp.MustCompile(`[^\p{L}\p{N} ._-]+`)

func extensao(d []byte) string {
	switch {
	case bytes.HasPrefix(d, []byte("%PDF")):
		return ".pdf"
	case bytes.HasPrefix(d, []byte("PK\x03\x04")):
		return ".epub"
	case bytes.HasPrefix(d, []byte{0xEF, 0xBB, 0xBF}) || isTexto(d):
		return ".txt"
	}
	return ".bin"
}

func isTexto(d []byte) bool {
	if len(d) > 512 {
		d = d[:512]
	}
	return !bytes.ContainsRune(d, 0)
}

// Baixar troca cada token por um livro cifrado na CDN (passo 5), extrai as
// chaves e decifra localmente (passo 7). Livros já baixados são pulados.
//
// D-014 (ataque de falha seletiva): o comportamento na rede é o mesmo, abram
// as chaves ou não. Primeiro todos os tokens pendentes são trocados na CDN;
// só depois os livros são decifrados. Uma chave que não abre vira erro na tela
// do cliente, nunca uma requisição diferente ao site ou à CDN.
func (c *Cliente) Baixar(ctx context.Context, cp *Compra) error {
	if cp.Resposta == nil {
		return errors.New("compra sem resposta do OT; use a recuperação com o e-mail")
	}
	toks, err := c.tokens(cp, cp.Resposta)
	if err != nil {
		return err
	}
	if len(cp.Baixados) != len(cp.Livros) {
		cp.Baixados = make([]string, len(cp.Livros))
	}
	cifrados := make([][]byte, len(cp.Livros))
	for i, id := range cp.Livros {
		if cp.Baixados[i] != "" {
			continue
		}
		if cifrados[i], err = c.baixarCDN(ctx, id, toks[i]); err != nil {
			return fmt.Errorf("livro %d: %w", id, err)
		}
	}
	ks, errChaves := c.chaves(ctx, cp)
	var falhas []string
	for i, id := range cp.Livros {
		if cifrados[i] == nil {
			continue
		}
		if errChaves != nil {
			falhas = append(falhas, cp.Titulos[i])
			continue
		}
		claro, err := protocolo.DecifrarLivro(ks[i], id, cifrados[i])
		if err != nil {
			falhas = append(falhas, cp.Titulos[i])
			continue
		}
		nome := strconv.FormatInt(id, 10) + " - " + strings.TrimSpace(naoNome.ReplaceAllString(cp.Titulos[i], "")) + extensao(claro)
		destino := c.caminho("biblioteca", nome)
		if err := gravar(destino, claro); err != nil {
			return err
		}
		cp.Baixados[i] = destino
	}
	if err := c.salvar(cp); err != nil {
		return err
	}
	if len(falhas) > 0 {
		return fmt.Errorf("não foi possível abrir: %s. A chave recebida não confere com o livro (RF12); "+
			"isso indica resposta do OT corrompida ou adulterada pelo site", strings.Join(falhas, "; "))
	}
	// Livro baixado antes cujo arquivo saiu da biblioteca: o token já foi
	// usado (RF17) e não há segundo download. Avisa em vez de apontar para um
	// arquivo que não existe; nenhuma requisição a mais é feita.
	if a := ausentes(cp); len(a) > 0 {
		return fmt.Errorf("já baixado antes, mas o arquivo não está mais na biblioteca: %s. "+
			"Cada token vale um único download (RF17), então não é possível baixar de novo", strings.Join(a, "; "))
	}
	return nil
}

// ausentes lista os títulos marcados como baixados cujo arquivo não existe.
func ausentes(cp *Compra) []string {
	var t []string
	for i, arq := range cp.Baixados {
		if arq == "" {
			continue
		}
		if _, err := os.Stat(arq); err != nil {
			t = append(t, cp.Titulos[i])
		}
	}
	return t
}

// Biblioteca é a pasta dos livros decifrados.
func (c *Cliente) Biblioteca() string { return c.caminho("biblioteca") }

func (c *Cliente) baixarCDN(ctx context.Context, id int64, t protocolo.Token) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(c.CDNURL, "/")+"/livros/"+strconv.FormatInt(id, 10), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+t.String())
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CDN respondeu %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return b, nil
}

// Reenviar pede ao site o reenvio da resposta do OT ao e-mail da compra (RF15).
func (c *Cliente) Reenviar(ctx context.Context, compraID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.SiteURL, "/")+"/api/compras/"+compraID+"/reenvio", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var m struct{ Mensagem string }
	json.NewDecoder(resp.Body).Decode(&m)
	return m.Mensagem, nil
}

func preco(c int) string { return fmt.Sprintf("R$ %d,%02d", c/100, c%100) }

// Preco formata centavos.
func Preco(c int) string { return preco(c) }
