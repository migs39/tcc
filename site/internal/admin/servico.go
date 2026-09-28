// Package admin implementa as contas de administradores e a autenticação da
// área administrativa (RF01, UC01).
package admin

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrCredenciaisInvalidas = errors.New("e-mail ou senha inválidos")
	ErrEmailEmUso           = errors.New("já existe um administrador com este e-mail")
	ErrEmailInvalido        = errors.New("e-mail inválido")
	ErrNomeVazio            = errors.New("informe o nome")
	ErrSenhaCurta           = fmt.Errorf("a senha precisa ter pelo menos %d caracteres", TamanhoMinimoSenha)
	ErrNaoEncontrado        = errors.New("administrador não encontrado")
	ErrUltimoAdminAtivo     = errors.New("não é possível desativar o último administrador ativo")
	ErrJaExisteAdmin        = errors.New("já existe administrador cadastrado")
)

type Admin struct {
	ID       int64
	Nome     string
	Email    string
	Ativo    bool
	CriadoEm time.Time
}

type Servico struct {
	pool *pgxpool.Pool
	// hashFicticio é verificado quando o e-mail não existe, para que o tempo de
	// resposta do login não revele quais e-mails estão cadastrados.
	hashFicticio string
}

func NovoServico(pool *pgxpool.Pool) (*Servico, error) {
	h, err := GerarHashSenha("senha-ficticia-para-tempo-constante")
	if err != nil {
		return nil, err
	}
	return &Servico{pool: pool, hashFicticio: h}, nil
}

func normalizarEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	end, err := mail.ParseAddress(email)
	if err != nil || end.Address != email {
		return "", ErrEmailInvalido
	}
	return email, nil
}

func validarDados(nome, email string) (string, string, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return "", "", ErrNomeVazio
	}
	email, err := normalizarEmail(email)
	if err != nil {
		return "", "", err
	}
	return nome, email, nil
}

func validarSenha(senha string) error {
	if len([]rune(senha)) < TamanhoMinimoSenha {
		return ErrSenhaCurta
	}
	return nil
}

func ehViolacaoUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Cadastrar cria um administrador (RF01). Só é chamado por um admin autenticado
// ou pelo comando de seed.
func (s *Servico) Cadastrar(ctx context.Context, nome, email, senha string) (Admin, error) {
	nome, email, err := validarDados(nome, email)
	if err != nil {
		return Admin{}, err
	}
	if err := validarSenha(senha); err != nil {
		return Admin{}, err
	}
	hash, err := GerarHashSenha(senha)
	if err != nil {
		return Admin{}, err
	}
	a := Admin{Nome: nome, Email: email, Ativo: true}
	err = s.pool.QueryRow(ctx,
		`INSERT INTO administradores (nome, email, senha_hash) VALUES ($1, $2, $3)
		 RETURNING id, criado_em`, nome, email, hash).Scan(&a.ID, &a.CriadoEm)
	if ehViolacaoUnica(err) {
		return Admin{}, ErrEmailEmUso
	}
	return a, err
}

// CadastrarPrimeiro cria o primeiro administrador; recusa se já houver algum.
func (s *Servico) CadastrarPrimeiro(ctx context.Context, nome, email, senha string) (Admin, error) {
	var existe bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM administradores)`).Scan(&existe); err != nil {
		return Admin{}, err
	}
	if existe {
		return Admin{}, ErrJaExisteAdmin
	}
	return s.Cadastrar(ctx, nome, email, senha)
}

// Autenticar confere e-mail e senha de um administrador ativo.
func (s *Servico) Autenticar(ctx context.Context, email, senha string) (Admin, error) {
	var a Admin
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT id, nome, email, ativo, criado_em, senha_hash FROM administradores
		 WHERE lower(email) = lower($1)`, strings.TrimSpace(email)).
		Scan(&a.ID, &a.Nome, &a.Email, &a.Ativo, &a.CriadoEm, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		VerificarSenha(senha, s.hashFicticio)
		return Admin{}, ErrCredenciaisInvalidas
	}
	if err != nil {
		return Admin{}, err
	}
	ok, err := VerificarSenha(senha, hash)
	if err != nil {
		return Admin{}, err
	}
	if !ok || !a.Ativo {
		return Admin{}, ErrCredenciaisInvalidas
	}
	return a, nil
}

func (s *Servico) Listar(ctx context.Context) ([]Admin, error) {
	linhas, err := s.pool.Query(ctx,
		`SELECT id, nome, email, ativo, criado_em FROM administradores ORDER BY nome, id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(linhas, func(l pgx.CollectableRow) (Admin, error) {
		var a Admin
		err := l.Scan(&a.ID, &a.Nome, &a.Email, &a.Ativo, &a.CriadoEm)
		return a, err
	})
}

func (s *Servico) Buscar(ctx context.Context, id int64) (Admin, error) {
	var a Admin
	err := s.pool.QueryRow(ctx,
		`SELECT id, nome, email, ativo, criado_em FROM administradores WHERE id = $1`, id).
		Scan(&a.ID, &a.Nome, &a.Email, &a.Ativo, &a.CriadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, ErrNaoEncontrado
	}
	return a, err
}

// Atualizar altera nome e e-mail (gestão de perfis, RF01).
func (s *Servico) Atualizar(ctx context.Context, id int64, nome, email string) error {
	nome, email, err := validarDados(nome, email)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE administradores SET nome = $2, email = $3, atualizado_em = now() WHERE id = $1`,
		id, nome, email)
	if ehViolacaoUnica(err) {
		return ErrEmailEmUso
	}
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNaoEncontrado
	}
	return err
}

// TrocarSenha define uma nova senha e encerra todas as sessões daquele admin.
func (s *Servico) TrocarSenha(ctx context.Context, id int64, novaSenha string) error {
	if err := validarSenha(novaSenha); err != nil {
		return err
	}
	hash, err := GerarHashSenha(novaSenha)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE administradores SET senha_hash = $2, atualizado_em = now() WHERE id = $1`, id, hash)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNaoEncontrado
		}
		_, err = tx.Exec(ctx, `DELETE FROM sessoes_admin WHERE admin_id = $1`, id)
		return err
	})
}

// Desativar impede o login do admin e encerra suas sessões. O registro é
// mantido. Sempre sobra pelo menos um admin ativo.
func (s *Servico) Desativar(ctx context.Context, id int64) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Trava os admins ativos para que duas desativações simultâneas não
		// deixem o sistema sem nenhum.
		linhas, err := tx.Query(ctx, `SELECT id FROM administradores WHERE ativo FOR UPDATE`)
		if err != nil {
			return err
		}
		ativos, err := pgx.CollectRows(linhas, pgx.RowTo[int64])
		if err != nil {
			return err
		}
		alvoAtivo := false
		for _, a := range ativos {
			if a == id {
				alvoAtivo = true
			}
		}
		if !alvoAtivo {
			if _, err := s.buscarTx(ctx, tx, id); err != nil {
				return err
			}
			return nil // já inativo
		}
		if len(ativos) <= 1 {
			return ErrUltimoAdminAtivo
		}
		if _, err := tx.Exec(ctx,
			`UPDATE administradores SET ativo = FALSE, atualizado_em = now() WHERE id = $1`, id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM sessoes_admin WHERE admin_id = $1`, id)
		return err
	})
}

func (s *Servico) Reativar(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE administradores SET ativo = TRUE, atualizado_em = now() WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNaoEncontrado
	}
	return err
}

func (s *Servico) buscarTx(ctx context.Context, tx pgx.Tx, id int64) (Admin, error) {
	var a Admin
	err := tx.QueryRow(ctx,
		`SELECT id, nome, email, ativo, criado_em FROM administradores WHERE id = $1`, id).
		Scan(&a.ID, &a.Nome, &a.Email, &a.Ativo, &a.CriadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, ErrNaoEncontrado
	}
	return a, err
}
