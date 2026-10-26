package catalogo

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"tcc/site/internal/db/dbteste"
)

type ambiente struct {
	ctx  context.Context
	pool *pgxpool.Pool
	svc  *Servico
	cat  int64
}

func novoAmbiente(t *testing.T) ambiente {
	t.Helper()
	pool := dbteste.Novo(t)
	a := ambiente{ctx: context.Background(), pool: pool, svc: NovoServico(pool)}
	c, err := a.svc.CriarCategoria(a.ctx, "Ficção")
	if err != nil {
		t.Fatal(err)
	}
	a.cat = c.ID
	return a
}

func (a ambiente) faixa(t *testing.T, centavos int) int64 {
	t.Helper()
	f, err := a.svc.CriarFaixa(a.ctx, centavos)
	if err != nil {
		t.Fatal(err)
	}
	return f.ID
}

func (a ambiente) livro(t *testing.T, titulo string, faixa int64) int64 {
	t.Helper()
	id, err := a.svc.Cadastrar(a.ctx, DadosLivro{Titulo: titulo, Autor: "Autor", CategoriaID: a.cat, FaixaID: faixa})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (a ambiente) versao(t *testing.T, faixa int64) VersaoGrupo {
	t.Helper()
	v, err := a.svc.VersaoAtual(a.ctx, faixa)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// RF03: o preço de um livro precisa estar no conjunto pré-definido.
func TestRF03PrecoForaDoConjunto(t *testing.T) {
	a := novoAmbiente(t)

	_, err := a.svc.Cadastrar(a.ctx, DadosLivro{Titulo: "T", Autor: "A", CategoriaID: a.cat, FaixaID: 999})
	if !errors.Is(err, ErrPrecoForaDoConjunto) {
		t.Fatalf("faixa inexistente: esperado ErrPrecoForaDoConjunto, veio %v", err)
	}

	// O banco também recusa, mesmo contornando o serviço.
	_, err = a.pool.Exec(a.ctx,
		`INSERT INTO livros (titulo, autor, categoria_id, faixa_id) VALUES ('T', 'A', $1, 999)`, a.cat)
	if codigoPg(err) != "23503" {
		t.Fatalf("banco aceitou livro com faixa inexistente: %v", err)
	}

	// Faixa inativa não recebe livros novos.
	f := a.faixa(t, 1990)
	if err := a.svc.DesativarFaixa(a.ctx, f); err != nil {
		t.Fatal(err)
	}
	_, err = a.svc.Cadastrar(a.ctx, DadosLivro{Titulo: "T", Autor: "A", CategoriaID: a.cat, FaixaID: f})
	if !errors.Is(err, ErrFaixaInativa) {
		t.Fatalf("faixa inativa: esperado ErrFaixaInativa, veio %v", err)
	}
}

func TestFaixas(t *testing.T) {
	a := novoAmbiente(t)
	f := a.faixa(t, 2990)
	if _, err := a.svc.CriarFaixa(a.ctx, 2990); !errors.Is(err, ErrFaixaDuplicada) {
		t.Fatalf("faixa duplicada: %v", err)
	}
	l := a.livro(t, "Livro", f)
	if err := a.svc.DesativarFaixa(a.ctx, f); !errors.Is(err, ErrFaixaComLivros) {
		t.Fatalf("desativou faixa com livro ativo: %v", err)
	}
	if err := a.svc.Desativar(a.ctx, l); err != nil {
		t.Fatal(err)
	}
	if err := a.svc.DesativarFaixa(a.ctx, f); err != nil {
		t.Fatalf("não desativou faixa sem livros ativos: %v", err)
	}
	if err := a.svc.Reativar(a.ctx, l); !errors.Is(err, ErrFaixaInativa) {
		t.Fatalf("reativou livro em faixa inativa: %v", err)
	}
}

func TestCategorias(t *testing.T) {
	a := novoAmbiente(t)
	if _, err := a.svc.CriarCategoria(a.ctx, "  ficção "); !errors.Is(err, ErrCategoriaDuplicada) {
		t.Fatalf("categoria duplicada (maiúsculas/espaços): %v", err)
	}
	if _, err := a.svc.CriarCategoria(a.ctx, " "); !errors.Is(err, ErrCategoriaVazia) {
		t.Fatalf("categoria vazia: %v", err)
	}
	a.livro(t, "Livro", a.faixa(t, 990))
	if err := a.svc.RemoverCategoria(a.ctx, a.cat); !errors.Is(err, ErrCategoriaEmUso) {
		t.Fatalf("removeu categoria em uso: %v", err)
	}
	vazia, _ := a.svc.CriarCategoria(a.ctx, "Poesia")
	if err := a.svc.RemoverCategoria(a.ctx, vazia.ID); err != nil {
		t.Fatal(err)
	}
}

func TestValidacaoLivro(t *testing.T) {
	a := novoAmbiente(t)
	f := a.faixa(t, 990)
	base := DadosLivro{Titulo: "T", Autor: "A", CategoriaID: a.cat, FaixaID: f}
	casos := []struct {
		mudar func(*DadosLivro)
		erro  error
	}{
		{func(d *DadosLivro) { d.Titulo = "  " }, ErrTituloVazio},
		{func(d *DadosLivro) { d.Autor = "" }, ErrAutorVazio},
		{func(d *DadosLivro) { d.Ano = 9999 }, ErrAnoInvalido},
		{func(d *DadosLivro) { d.ISBN = "123" }, ErrISBNInvalido},
		{func(d *DadosLivro) { d.CategoriaID = 0 }, ErrCategoriaInvalida},
		{func(d *DadosLivro) { d.CategoriaID = 999 }, ErrCategoriaInvalida},
	}
	for i, c := range casos {
		d := base
		c.mudar(&d)
		if _, err := a.svc.Cadastrar(a.ctx, d); !errors.Is(err, c.erro) {
			t.Errorf("caso %d: esperado %v, veio %v", i, c.erro, err)
		}
	}

	d := base
	d.ISBN, d.Ano, d.Editora, d.Descricao = "978-0-306-40615-7", 2020, "Editora X", "Sinopse"
	id, err := a.svc.Cadastrar(a.ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := a.svc.Buscar(a.ctx, id)
	if l.ISBN != "9780306406157" || l.Ano != 2020 || l.PrecoCentavos != 990 || l.Categoria != "Ficção" {
		t.Fatalf("livro gravado errado: %+v", l)
	}
}

// RF07 / L2: a composição do grupo é congelada em versões; só mudanças de
// composição criam versão nova, e versões antigas não mudam.
func TestVersoesDeGrupo(t *testing.T) {
	a := novoAmbiente(t)
	fa, fb := a.faixa(t, 2990), a.faixa(t, 3990)

	l1 := a.livro(t, "Um", fa)
	l2 := a.livro(t, "Dois", fa)
	v := a.versao(t, fa)
	if v.Numero != 2 || !slices.Equal(v.Livros, []int64{l1, l2}) || v.PrecoCentavos != 2990 {
		t.Fatalf("após dois cadastros: %+v", v)
	}
	congelada := v

	// Editar dados sem mudar o preço não versiona.
	if err := a.svc.Atualizar(a.ctx, l1, DadosLivro{Titulo: "Um (2ª ed.)", Autor: "Autor", CategoriaID: a.cat, FaixaID: fa}); err != nil {
		t.Fatal(err)
	}
	if v := a.versao(t, fa); v.Numero != 2 {
		t.Fatalf("edição de título criou versão: %+v", v)
	}

	// Remover do catálogo versiona.
	if err := a.svc.Desativar(a.ctx, l1); err != nil {
		t.Fatal(err)
	}
	if v := a.versao(t, fa); v.Numero != 3 || !slices.Equal(v.Livros, []int64{l2}) {
		t.Fatalf("após desativar: %+v", v)
	}
	// Desativar de novo não muda nada.
	if err := a.svc.Desativar(a.ctx, l1); err != nil {
		t.Fatal(err)
	}
	if v := a.versao(t, fa); v.Numero != 3 {
		t.Fatalf("desativação repetida criou versão: %+v", v)
	}

	// Trocar de preço versiona os dois grupos.
	if err := a.svc.Atualizar(a.ctx, l2, DadosLivro{Titulo: "Dois", Autor: "Autor", CategoriaID: a.cat, FaixaID: fb}); err != nil {
		t.Fatal(err)
	}
	if v := a.versao(t, fa); v.Numero != 4 || len(v.Livros) != 0 {
		t.Fatalf("grupo de origem após mudança de preço: %+v", v)
	}
	if v := a.versao(t, fb); v.Numero != 1 || !slices.Equal(v.Livros, []int64{l2}) {
		t.Fatalf("grupo de destino após mudança de preço: %+v", v)
	}

	// A versão 2 continua igual (imutável).
	var ids []int64
	linhas, _ := a.pool.Query(a.ctx, `SELECT livro_id FROM versao_grupo_livros WHERE versao_id = $1 ORDER BY indice`, congelada.ID)
	for linhas.Next() {
		var id int64
		linhas.Scan(&id)
		ids = append(ids, id)
	}
	if !slices.Equal(ids, congelada.Livros) {
		t.Fatalf("versão antiga mudou: %v -> %v", congelada.Livros, ids)
	}
}

func pngValido(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCapa(t *testing.T) {
	a := novoAmbiente(t)
	l := a.livro(t, "Com capa", a.faixa(t, 990))

	if err := a.svc.DefinirCapa(a.ctx, l, []byte("isto não é uma imagem")); !errors.Is(err, ErrCapaTipo) {
		t.Fatalf("aceitou capa que não é imagem: %v", err)
	}
	if err := a.svc.DefinirCapa(a.ctx, l, make([]byte, TamanhoMaximoCapa+1)); !errors.Is(err, ErrCapaTamanho) {
		t.Fatalf("aceitou capa grande demais: %v", err)
	}
	img := pngValido(t)
	if err := a.svc.DefinirCapa(a.ctx, l, img); err != nil {
		t.Fatal(err)
	}
	tipo, dados, _, err := a.svc.Capa(a.ctx, l)
	if err != nil || tipo != "image/png" || !bytes.Equal(dados, img) {
		t.Fatalf("capa lida errada: %q %v", tipo, err)
	}
	if err := a.svc.DefinirCapa(a.ctx, 999, img); !errors.Is(err, ErrNaoEncontrado) {
		t.Fatalf("capa para livro inexistente: %v", err)
	}

	// D-006: miniatura de tamanho fixo em JPEG, qualquer que seja a original.
	mini, _, err := a.svc.Miniatura(a.ctx, l)
	if err != nil {
		t.Fatal(err)
	}
	cfg, formato, err := image.DecodeConfig(bytes.NewReader(mini))
	if err != nil || formato != "jpeg" || cfg.Width != LarguraMiniatura || cfg.Height != AlturaMiniatura {
		t.Fatalf("miniatura: %s %dx%d %v", formato, cfg.Width, cfg.Height, err)
	}

	// Livro fora do catálogo sai do pacote: sem miniatura. O admin ainda vê a original.
	a.svc.Desativar(a.ctx, l)
	if _, _, err := a.svc.Miniatura(a.ctx, l); !errors.Is(err, ErrNaoEncontrado) {
		t.Fatalf("miniatura de livro inativo servida: %v", err)
	}
	if _, _, _, err := a.svc.Capa(a.ctx, l); err != nil {
		t.Fatalf("admin não vê capa de livro inativo: %v", err)
	}
}

// Miniaturas têm sempre as mesmas dimensões, para capas largas, altas ou minúsculas.
func TestGerarMiniatura(t *testing.T) {
	for _, r := range []image.Rectangle{
		image.Rect(0, 0, 1, 1), image.Rect(0, 0, 3000, 200), image.Rect(0, 0, 150, 4000), image.Rect(0, 0, 400, 600),
	} {
		var buf bytes.Buffer
		png.Encode(&buf, image.NewRGBA(r))
		mini, err := GerarMiniatura(buf.Bytes())
		if err != nil {
			t.Fatalf("%v: %v", r, err)
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(mini))
		if err != nil || cfg.Width != LarguraMiniatura || cfg.Height != AlturaMiniatura {
			t.Errorf("%v: miniatura %dx%d %v", r, cfg.Width, cfg.Height, err)
		}
	}
	if _, err := GerarMiniatura([]byte("\x89PNG\r\n\x1a\nlixo")); err == nil {
		t.Error("aceitou PNG corrompido")
	}
}
