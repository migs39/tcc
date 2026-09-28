package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// DuracaoSessao é o tempo de vida de uma sessão de administrador.
const DuracaoSessao = 8 * time.Hour

var ErrSessaoInvalida = errors.New("sessão inválida ou expirada")

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// CriarSessao gera um token de 256 bits (CSPRNG) e guarda apenas o hash dele.
func (s *Servico) CriarSessao(ctx context.Context, adminID int64) (token string, expira time.Time, err error) {
	bruto := make([]byte, 32)
	if _, err := rand.Read(bruto); err != nil {
		return "", time.Time{}, err
	}
	token = base64.RawURLEncoding.EncodeToString(bruto)
	expira = time.Now().Add(DuracaoSessao)
	_, err = s.pool.Exec(ctx,
		`INSERT INTO sessoes_admin (token_hash, admin_id, expira_em) VALUES ($1, $2, $3)`,
		hashToken(token), adminID, expira)
	if err != nil {
		return "", time.Time{}, err
	}
	// Limpeza oportunista de sessões vencidas.
	s.pool.Exec(ctx, `DELETE FROM sessoes_admin WHERE expira_em < now()`)
	return token, expira, nil
}

// AdminDaSessao devolve o admin dono de uma sessão válida e ainda ativo.
func (s *Servico) AdminDaSessao(ctx context.Context, token string) (Admin, error) {
	if token == "" {
		return Admin{}, ErrSessaoInvalida
	}
	var a Admin
	err := s.pool.QueryRow(ctx,
		`SELECT a.id, a.nome, a.email, a.ativo, a.criado_em
		   FROM sessoes_admin s JOIN administradores a ON a.id = s.admin_id
		  WHERE s.token_hash = $1 AND s.expira_em > now() AND a.ativo`,
		hashToken(token)).Scan(&a.ID, &a.Nome, &a.Email, &a.Ativo, &a.CriadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, ErrSessaoInvalida
	}
	return a, err
}

func (s *Servico) EncerrarSessao(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessoes_admin WHERE token_hash = $1`, hashToken(token))
	return err
}
