// Package catalogo implementa o catálogo de livros (RF02), a política de preços
// discretos (RF03) e as versões imutáveis dos grupos de mesmo preço (RF07, L2).
package catalogo

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TamanhoMaximoCapa é o limite da imagem de capa (2 MiB).
const TamanhoMaximoCapa = 2 << 20

var (
	ErrNaoEncontrado       = errors.New("registro não encontrado")
	ErrFaixaDuplicada      = errors.New("já existe uma faixa com este preço")
	ErrFaixaInativa        = errors.New("a faixa de preço escolhida não está ativa")
	ErrFaixaComLivros      = errors.New("a faixa tem livros ativos; mude o preço deles antes de desativá-la")
	ErrCategoriaDuplicada  = errors.New("já existe uma categoria com este nome")
	ErrCategoriaEmUso      = errors.New("a categoria está em uso por algum livro")
	ErrCategoriaVazia      = errors.New("informe o nome da categoria")
	ErrTituloVazio         = errors.New("informe o título")
	ErrAutorVazio          = errors.New("informe o autor")
	ErrAnoInvalido         = errors.New("ano inválido")
	ErrCategoriaInvalida   = errors.New("escolha uma categoria")
	ErrPrecoForaDoConjunto = errors.New("escolha um preço do conjunto pré-definido")
	ErrCapaTipo            = errors.New("a capa precisa ser JPEG, PNG ou WebP")
	ErrCapaTamanho         = errors.New("a capa precisa ter até 2 MB")
)

// ErroDeValidacao diz se o erro deve ser mostrado ao admin (os demais viram
// "erro interno").
func ErroDeValidacao(err error) bool {
	for _, e := range []error{
		ErrFaixaDuplicada, ErrFaixaInativa, ErrFaixaComLivros, ErrCategoriaDuplicada,
		ErrCategoriaEmUso, ErrCategoriaVazia, ErrTituloVazio, ErrAutorVazio, ErrAnoInvalido,
		ErrCategoriaInvalida, ErrPrecoForaDoConjunto, ErrCapaTipo, ErrCapaTamanho,
		ErrPrecoInvalido, ErrISBNInvalido, ErrCapaIlegivel,
	} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

type Faixa struct {
	ID            int64
	ValorCentavos int
	Ativa         bool
	LivrosAtivos  int
}

type Categoria struct {
	ID     int64
	Nome   string
	Livros int
}

// DadosLivro são os campos editáveis de um livro (RF02).
type DadosLivro struct {
	Titulo      string
	Autor       string
	Descricao   string
	Ano         int // 0 = não informado
	ISBN        string
	Editora     string
	CategoriaID int64
	FaixaID     int64
}

type Livro struct {
	ID int64
	DadosLivro
	Categoria     string
	PrecoCentavos int
	Ativo         bool
	TemCapa       bool
}

// VersaoGrupo é uma composição congelada de um grupo de mesmo preço. O índice
// de cada livro no OT é sua posição em Livros.
type VersaoGrupo struct {
	ID            int64
	FaixaID       int64
	Numero        int
	PrecoCentavos int
	Livros        []int64
}

type Servico struct {
	pool *pgxpool.Pool
	// agora permite fixar o relógio nos testes.
	agora func() time.Time
}

func NovoServico(pool *pgxpool.Pool) *Servico {
	return &Servico{pool: pool, agora: time.Now}
}

func codigoPg(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// --- faixas de preço (RF03) ------------------------------------------------

func (s *Servico) CriarFaixa(ctx context.Context, valorCentavos int) (Faixa, error) {
	if valorCentavos <= 0 {
		return Faixa{}, ErrPrecoInvalido
	}
	f := Faixa{ValorCentavos: valorCentavos, Ativa: true}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO faixas_preco (valor_centavos) VALUES ($1) RETURNING id`, valorCentavos).Scan(&f.ID)
	if codigoPg(err) == "23505" {
		return Faixa{}, ErrFaixaDuplicada
	}
	return f, err
}

func (s *Servico) ListarFaixas(ctx context.Context) ([]Faixa, error) {
	linhas, err := s.pool.Query(ctx,
		`SELECT f.id, f.valor_centavos, f.ativa,
		        (SELECT count(*) FROM livros l WHERE l.faixa_id = f.id AND l.ativo)
		   FROM faixas_preco f ORDER BY f.valor_centavos`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(linhas, func(l pgx.CollectableRow) (Faixa, error) {
		var f Faixa
		err := l.Scan(&f.ID, &f.ValorCentavos, &f.Ativa, &f.LivrosAtivos)
		return f, err
	})
}

// DesativarFaixa tira a faixa do conjunto oferecido. Só é possível sem livros
// ativos nela, para nenhum livro ficar com preço fora do conjunto.
func (s *Servico) DesativarFaixa(ctx context.Context, id int64) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := travarFaixas(ctx, tx, id); err != nil {
			return err
		}
		var ativos int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM livros WHERE faixa_id = $1 AND ativo`, id).Scan(&ativos); err != nil {
			return err
		}
		if ativos > 0 {
			return ErrFaixaComLivros
		}
		_, err := tx.Exec(ctx, `UPDATE faixas_preco SET ativa = FALSE WHERE id = $1`, id)
		return err
	})
}

func (s *Servico) ReativarFaixa(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE faixas_preco SET ativa = TRUE WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNaoEncontrado
	}
	return err
}

// travarFaixas trava as faixas em ordem crescente de id (evita deadlock entre
// transações que mexem em duas faixas) e confirma que existem.
func travarFaixas(ctx context.Context, tx pgx.Tx, ids ...int64) error {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for _, id := range ids {
		var x int64
		err := tx.QueryRow(ctx, `SELECT id FROM faixas_preco WHERE id = $1 FOR UPDATE`, id).Scan(&x)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPrecoForaDoConjunto
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func faixaAtiva(ctx context.Context, tx pgx.Tx, id int64) error {
	var ativa bool
	err := tx.QueryRow(ctx, `SELECT ativa FROM faixas_preco WHERE id = $1`, id).Scan(&ativa)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPrecoForaDoConjunto
	}
	if err != nil {
		return err
	}
	if !ativa {
		return ErrFaixaInativa
	}
	return nil
}

// --- categorias ------------------------------------------------------------

func (s *Servico) CriarCategoria(ctx context.Context, nome string) (Categoria, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return Categoria{}, ErrCategoriaVazia
	}
	c := Categoria{Nome: nome}
	err := s.pool.QueryRow(ctx, `INSERT INTO categorias (nome) VALUES ($1) RETURNING id`, nome).Scan(&c.ID)
	if codigoPg(err) == "23505" {
		return Categoria{}, ErrCategoriaDuplicada
	}
	return c, err
}

func (s *Servico) ListarCategorias(ctx context.Context) ([]Categoria, error) {
	linhas, err := s.pool.Query(ctx,
		`SELECT c.id, c.nome, (SELECT count(*) FROM livros l WHERE l.categoria_id = c.id)
		   FROM categorias c ORDER BY lower(c.nome)`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(linhas, func(l pgx.CollectableRow) (Categoria, error) {
		var c Categoria
		err := l.Scan(&c.ID, &c.Nome, &c.Livros)
		return c, err
	})
}

func (s *Servico) RenomearCategoria(ctx context.Context, id int64, nome string) error {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return ErrCategoriaVazia
	}
	tag, err := s.pool.Exec(ctx, `UPDATE categorias SET nome = $2 WHERE id = $1`, id, nome)
	if codigoPg(err) == "23505" {
		return ErrCategoriaDuplicada
	}
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNaoEncontrado
	}
	return err
}

// RemoverCategoria só apaga categorias sem nenhum livro (ativo ou não).
func (s *Servico) RemoverCategoria(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM categorias WHERE id = $1`, id)
	if codigoPg(err) == "23503" {
		return ErrCategoriaEmUso
	}
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNaoEncontrado
	}
	return err
}

// --- livros (RF02) ---------------------------------------------------------

func (s *Servico) validar(d DadosLivro) (DadosLivro, error) {
	d.Titulo = strings.TrimSpace(d.Titulo)
	d.Autor = strings.TrimSpace(d.Autor)
	d.Descricao = strings.TrimSpace(d.Descricao)
	d.Editora = strings.TrimSpace(d.Editora)
	switch {
	case d.Titulo == "":
		return d, ErrTituloVazio
	case d.Autor == "":
		return d, ErrAutorVazio
	case d.Ano < 0 || d.Ano > s.agora().Year()+1:
		return d, ErrAnoInvalido
	case d.CategoriaID <= 0:
		return d, ErrCategoriaInvalida
	case d.FaixaID <= 0:
		return d, ErrPrecoForaDoConjunto
	}
	isbn, err := NormalizarISBN(d.ISBN)
	if err != nil {
		return d, err
	}
	d.ISBN = isbn
	return d, nil
}

func nulo[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}

// Cadastrar cria um livro ativo e gera nova versão do grupo da sua faixa.
func (s *Servico) Cadastrar(ctx context.Context, d DadosLivro) (int64, error) {
	d, err := s.validar(d)
	if err != nil {
		return 0, err
	}
	var id int64
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := travarFaixas(ctx, tx, d.FaixaID); err != nil {
			return err
		}
		if err := faixaAtiva(ctx, tx, d.FaixaID); err != nil {
			return err
		}
		err := tx.QueryRow(ctx,
			`INSERT INTO livros (titulo, autor, descricao, ano, isbn, editora, categoria_id, faixa_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			d.Titulo, d.Autor, d.Descricao, nulo(d.Ano), nulo(d.ISBN), d.Editora, d.CategoriaID, d.FaixaID).Scan(&id)
		if codigoPg(err) == "23503" {
			return ErrCategoriaInvalida
		}
		if err != nil {
			return err
		}
		return novaVersao(ctx, tx, d.FaixaID)
	})
	return id, err
}

// Atualizar altera os dados do livro. Só trocar de faixa (preço) muda a
// composição dos grupos; editar título, descrição etc. não gera versão.
func (s *Servico) Atualizar(ctx context.Context, id int64, d DadosLivro) error {
	d, err := s.validar(d)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var faixaAntiga int64
		var ativo bool
		err := tx.QueryRow(ctx, `SELECT faixa_id, ativo FROM livros WHERE id = $1 FOR UPDATE`, id).
			Scan(&faixaAntiga, &ativo)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNaoEncontrado
		}
		if err != nil {
			return err
		}
		mudouFaixa := faixaAntiga != d.FaixaID
		if mudouFaixa {
			if err := travarFaixas(ctx, tx, faixaAntiga, d.FaixaID); err != nil {
				return err
			}
			if err := faixaAtiva(ctx, tx, d.FaixaID); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx,
			`UPDATE livros SET titulo = $2, autor = $3, descricao = $4, ano = $5, isbn = $6,
			        editora = $7, categoria_id = $8, faixa_id = $9, atualizado_em = now()
			  WHERE id = $1`,
			id, d.Titulo, d.Autor, d.Descricao, nulo(d.Ano), nulo(d.ISBN), d.Editora, d.CategoriaID, d.FaixaID)
		if codigoPg(err) == "23503" {
			return ErrCategoriaInvalida
		}
		if err != nil {
			return err
		}
		if mudouFaixa && ativo {
			if err := novaVersao(ctx, tx, faixaAntiga); err != nil {
				return err
			}
			return novaVersao(ctx, tx, d.FaixaID)
		}
		return nil
	})
}

// Desativar tira o livro do catálogo ("remover", RF02) e gera nova versão do
// grupo. O registro fica: versões antigas e compras feitas sobre elas o usam.
func (s *Servico) Desativar(ctx context.Context, id int64) error {
	return s.mudarAtivo(ctx, id, false)
}

// Reativar devolve o livro ao catálogo; a faixa dele precisa estar ativa.
func (s *Servico) Reativar(ctx context.Context, id int64) error {
	return s.mudarAtivo(ctx, id, true)
}

func (s *Servico) mudarAtivo(ctx context.Context, id int64, ativo bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var faixa int64
		var atual bool
		err := tx.QueryRow(ctx, `SELECT faixa_id, ativo FROM livros WHERE id = $1 FOR UPDATE`, id).Scan(&faixa, &atual)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNaoEncontrado
		}
		if err != nil {
			return err
		}
		if atual == ativo {
			return nil
		}
		if err := travarFaixas(ctx, tx, faixa); err != nil {
			return err
		}
		if ativo {
			if err := faixaAtiva(ctx, tx, faixa); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx,
			`UPDATE livros SET ativo = $2, atualizado_em = now() WHERE id = $1`, id, ativo); err != nil {
			return err
		}
		return novaVersao(ctx, tx, faixa)
	})
}

const selectLivro = `SELECT l.id, l.titulo, l.autor, l.descricao, coalesce(l.ano, 0), coalesce(l.isbn, ''),
       l.editora, l.categoria_id, l.faixa_id, c.nome, f.valor_centavos, l.ativo,
       EXISTS (SELECT 1 FROM capas WHERE livro_id = l.id)
  FROM livros l
  JOIN categorias c ON c.id = l.categoria_id
  JOIN faixas_preco f ON f.id = l.faixa_id`

func lerLivro(l pgx.CollectableRow) (Livro, error) {
	var v Livro
	err := l.Scan(&v.ID, &v.Titulo, &v.Autor, &v.Descricao, &v.Ano, &v.ISBN, &v.Editora,
		&v.CategoriaID, &v.FaixaID, &v.Categoria, &v.PrecoCentavos, &v.Ativo, &v.TemCapa)
	return v, err
}

func (s *Servico) Buscar(ctx context.Context, id int64) (Livro, error) {
	linhas, err := s.pool.Query(ctx, selectLivro+` WHERE l.id = $1`, id)
	if err != nil {
		return Livro{}, err
	}
	v, err := pgx.CollectExactlyOneRow(linhas, lerLivro)
	if errors.Is(err, pgx.ErrNoRows) {
		return Livro{}, ErrNaoEncontrado
	}
	return v, err
}

// Listar devolve todos os livros (ativos e inativos), para a administração.
func (s *Servico) Listar(ctx context.Context) ([]Livro, error) {
	linhas, err := s.pool.Query(ctx, selectLivro+` ORDER BY lower(l.titulo), l.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(linhas, lerLivro)
}

// --- capas -----------------------------------------------------------------

// DefinirCapa grava a capa depois de conferir o tipo pelo conteúdo (não pelo
// nome do arquivo nem pelo Content-Type enviado).
func (s *Servico) DefinirCapa(ctx context.Context, livroID int64, dados []byte) error {
	if len(dados) == 0 || len(dados) > TamanhoMaximoCapa {
		return ErrCapaTamanho
	}
	tipo := http.DetectContentType(dados)
	if tipo != "image/jpeg" && tipo != "image/png" && tipo != "image/webp" {
		return ErrCapaTipo
	}
	mini, err := GerarMiniatura(dados)
	if err != nil {
		return err
	}
	h := sha256.Sum256(dados)
	hm := sha256.Sum256(mini)
	_, err = s.pool.Exec(ctx,
		`INSERT INTO capas (livro_id, tipo, dados, sha256, miniatura, miniatura_sha256)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (livro_id) DO UPDATE SET tipo = EXCLUDED.tipo, dados = EXCLUDED.dados,
		        sha256 = EXCLUDED.sha256, miniatura = EXCLUDED.miniatura,
		        miniatura_sha256 = EXCLUDED.miniatura_sha256`,
		livroID, tipo, dados, h[:], mini, hm[:])
	if codigoPg(err) == "23503" {
		return ErrNaoEncontrado
	}
	return err
}

func (s *Servico) RemoverCapa(ctx context.Context, livroID int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM capas WHERE livro_id = $1`, livroID)
	return err
}

// Capa devolve a imagem original, para a área administrativa. O público não
// recebe capas avulsas: o programa local baixa as miniaturas de todos os
// livros no pacote do catálogo (D-006).
func (s *Servico) Capa(ctx context.Context, livroID int64) (tipo string, dados, hash []byte, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT tipo, dados, sha256 FROM capas WHERE livro_id = $1`, livroID).Scan(&tipo, &dados, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNaoEncontrado
	}
	return
}

// Miniatura devolve a miniatura JPEG de tamanho fixo de um livro ativo.
func (s *Servico) Miniatura(ctx context.Context, livroID int64) (dados, hash []byte, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT c.miniatura, c.miniatura_sha256 FROM capas c JOIN livros l ON l.id = c.livro_id
		  WHERE c.livro_id = $1 AND l.ativo`, livroID).Scan(&dados, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNaoEncontrado
	}
	return
}

// --- versões de grupo (RF07, L2) -------------------------------------------

// novaVersao congela a composição atual do grupo da faixa. A ordem é por id do
// livro, estável e pública. Deve rodar com a faixa já travada.
func novaVersao(ctx context.Context, tx pgx.Tx, faixaID int64) error {
	var versaoID int64
	err := tx.QueryRow(ctx,
		`INSERT INTO versoes_grupo (faixa_id, numero)
		 SELECT $1, coalesce(max(numero), 0) + 1 FROM versoes_grupo WHERE faixa_id = $1
		 RETURNING id`, faixaID).Scan(&versaoID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO versao_grupo_livros (versao_id, indice, livro_id)
		 SELECT $1, row_number() OVER (ORDER BY id) - 1, id
		   FROM livros WHERE faixa_id = $2 AND ativo`, versaoID, faixaID)
	return err
}

// VersaoAtual devolve a versão mais recente do grupo de uma faixa.
func (s *Servico) VersaoAtual(ctx context.Context, faixaID int64) (VersaoGrupo, error) {
	v := VersaoGrupo{FaixaID: faixaID}
	err := s.pool.QueryRow(ctx,
		`SELECT v.id, v.numero, f.valor_centavos FROM versoes_grupo v
		   JOIN faixas_preco f ON f.id = v.faixa_id
		  WHERE v.faixa_id = $1 ORDER BY v.numero DESC LIMIT 1`, faixaID).
		Scan(&v.ID, &v.Numero, &v.PrecoCentavos)
	if errors.Is(err, pgx.ErrNoRows) {
		return VersaoGrupo{}, ErrNaoEncontrado
	}
	if err != nil {
		return VersaoGrupo{}, err
	}
	linhas, err := s.pool.Query(ctx,
		`SELECT livro_id FROM versao_grupo_livros WHERE versao_id = $1 ORDER BY indice`, v.ID)
	if err != nil {
		return VersaoGrupo{}, err
	}
	v.Livros, err = pgx.CollectRows(linhas, pgx.RowTo[int64])
	return v, err
}
