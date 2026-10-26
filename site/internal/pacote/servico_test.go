package pacote

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"tcc/site/internal/catalogo"
	"tcc/site/internal/db/dbteste"
)

func chaveNova(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv
}

// abrir devolve o catalogo.json e os arquivos do ZIP.
func abrir(t *testing.T, dados []byte) (Catalogo, map[string][]byte) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(dados), int64(len(dados)))
	if err != nil {
		t.Fatal(err)
	}
	arquivos := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		arquivos[f.Name] = b
	}
	var cat Catalogo
	if err := json.Unmarshal(arquivos["catalogo.json"], &cat); err != nil {
		t.Fatalf("catalogo.json: %v", err)
	}
	return cat, arquivos
}

type amb struct {
	ctx  context.Context
	pool *pgxpool.Pool
	cat  *catalogo.Servico
	catg int64
}

func novoAmb(t *testing.T) amb {
	t.Helper()
	pool := dbteste.Novo(t)
	a := amb{ctx: context.Background(), pool: pool, cat: catalogo.NovoServico(pool)}
	c, err := a.cat.CriarCategoria(a.ctx, "Ficção")
	if err != nil {
		t.Fatal(err)
	}
	a.catg = c.ID
	return a
}

func (a amb) livro(t *testing.T, titulo string, faixa int64) int64 {
	t.Helper()
	id, err := a.cat.Cadastrar(a.ctx, catalogo.DadosLivro{Titulo: titulo, Autor: "Autor", CategoriaID: a.catg, FaixaID: faixa})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func pngDe(t *testing.T) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 40, 60))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// RF04 / D-006: o pacote leva os livros ativos, as versões atuais dos grupos
// (na ordem do OT) e as miniaturas; nada de livros removidos.
func TestConteudoDoPacote(t *testing.T) {
	a := novoAmb(t)
	fa, _ := a.cat.CriarFaixa(a.ctx, 2990)
	vazia, _ := a.cat.CriarFaixa(a.ctx, 3990)
	l1 := a.livro(t, "Um", fa.ID)
	l2 := a.livro(t, "Dois", fa.ID)
	l3 := a.livro(t, "Três", fa.ID)
	if err := a.cat.Desativar(a.ctx, l3); err != nil {
		t.Fatal(err)
	}
	if err := a.cat.DefinirCapa(a.ctx, l1, pngDe(t)); err != nil {
		t.Fatal(err)
	}

	p, err := NovoServico(a.pool, chaveNova(t)).Atual(a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	cat, arquivos := abrir(t, p.Dados)

	if cat.Formato != FormatoAtual || len(cat.Categorias) != 1 {
		t.Fatalf("cabeçalho do catálogo: %+v", cat)
	}
	var ids []int64
	for _, l := range cat.Livros {
		ids = append(ids, l.ID)
	}
	if !slices.Equal(ids, []int64{l1, l2}) {
		t.Fatalf("livros no pacote: %v (esperado %v, sem o removido %d)", ids, []int64{l1, l2}, l3)
	}
	versao, _ := a.cat.VersaoAtual(a.ctx, fa.ID)
	if len(cat.Grupos) != 1 {
		t.Fatalf("grupos: %+v (faixa sem livros %d não deveria aparecer)", cat.Grupos, vazia.ID)
	}
	g := cat.Grupos[0]
	if g.FaixaID != fa.ID || g.VersaoGrupoID != versao.ID || g.VersaoNumero != versao.Numero ||
		g.PrecoCentavos != 2990 || !slices.Equal(g.Livros, versao.Livros) {
		t.Fatalf("grupo %+v difere da versão atual %+v", g, versao)
	}

	mini, _, _ := a.cat.Miniatura(a.ctx, l1)
	if cat.Livros[0].Miniatura == "" || !bytes.Equal(arquivos[cat.Livros[0].Miniatura], mini) {
		t.Fatal("miniatura do livro com capa ausente ou diferente")
	}
	if cat.Livros[1].Miniatura != "" {
		t.Fatal("livro sem capa com miniatura no pacote")
	}
	if len(arquivos) != 2 {
		t.Fatalf("arquivos no ZIP: %d (esperado catalogo.json + 1 miniatura)", len(arquivos))
	}
}

// Mesmo conteúdo, mesmos bytes e mesma versão; conteúdo novo, versão nova;
// assinatura confere e detecta adulteração.
func TestVersoesEAssinatura(t *testing.T) {
	a := novoAmb(t)
	fa, _ := a.cat.CriarFaixa(a.ctx, 990)
	l := a.livro(t, "Título", fa.ID)
	chave := chaveNova(t)
	pub := chave.Public().(ed25519.PublicKey)

	p1, err := NovoServico(a.pool, chave).Atual(a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p1.SHA256 != sha256.Sum256(p1.Dados) || !Verificar(pub, p1.Versao, p1.SHA256, p1.Assinatura) {
		t.Fatal("hash ou assinatura do pacote não conferem")
	}

	// Outro processo (sem cache) gera exatamente os mesmos bytes e reaproveita a versão.
	p2, _ := NovoServico(a.pool, chave).Atual(a.ctx)
	if p2.Versao != p1.Versao || !bytes.Equal(p2.Dados, p1.Dados) {
		t.Fatalf("pacote não determinístico: v%d vs v%d", p1.Versao, p2.Versao)
	}

	svc := NovoServico(a.pool, chave)
	svc.Atual(a.ctx)
	mudar := func(titulo string) {
		if err := a.cat.Atualizar(a.ctx, l, catalogo.DadosLivro{Titulo: titulo, Autor: "Autor", CategoriaID: a.catg, FaixaID: fa.ID}); err != nil {
			t.Fatal(err)
		}
	}
	mudar("Título novo")
	p3, _ := svc.Atual(a.ctx)
	if p3.Versao != p1.Versao+1 || p3.SHA256 == p1.SHA256 {
		t.Fatalf("mudança no catálogo não gerou versão nova: v%d", p3.Versao)
	}
	// Voltar ao conteúdo antigo não reaproveita versão antiga: a versão só cresce.
	mudar("Título")
	p4, _ := svc.Atual(a.ctx)
	if p4.Versao != p3.Versao+1 || p4.SHA256 != p1.SHA256 {
		t.Fatalf("volta ao conteúdo antigo: v%d (esperado v%d com o hash da v%d)", p4.Versao, p3.Versao+1, p1.Versao)
	}

	adulterado := bytes.Clone(p4.Dados)
	adulterado[len(adulterado)/2] ^= 1
	if Verificar(pub, p4.Versao, sha256.Sum256(adulterado), p4.Assinatura) {
		t.Fatal("assinatura aceitou pacote adulterado")
	}
	if Verificar(pub, p4.Versao-1, p4.SHA256, p4.Assinatura) {
		t.Fatal("assinatura aceitou número de versão trocado")
	}
	if Verificar(chaveNova(t).Public().(ed25519.PublicKey), p4.Versao, p4.SHA256, p4.Assinatura) {
		t.Fatal("assinatura aceitou outra chave pública")
	}

	// Trocar a chave do site gera versão nova assinada com a chave nova.
	outra := chaveNova(t)
	p5, _ := NovoServico(a.pool, outra).Atual(a.ctx)
	if p5.Versao != p4.Versao+1 || !Verificar(outra.Public().(ed25519.PublicKey), p5.Versao, p5.SHA256, p5.Assinatura) {
		t.Fatalf("troca de chave: v%d", p5.Versao)
	}
}

func TestCarregarOuCriarChave(t *testing.T) {
	arq := filepath.Join(t.TempDir(), "chave.ed25519")
	k1, criada, err := CarregarOuCriarChave(arq)
	if err != nil || !criada {
		t.Fatalf("criar: %v %v", criada, err)
	}
	k2, criada, err := CarregarOuCriarChave(arq)
	if err != nil || criada || !k1.Equal(k2) {
		t.Fatalf("recarregar: %v %v", criada, err)
	}
	os.WriteFile(arq, []byte("não é hex"), 0o600)
	if _, _, err := CarregarOuCriarChave(arq); err == nil {
		t.Fatal("aceitou arquivo de chave inválido")
	}
}
