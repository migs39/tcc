package pacote

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pacote é uma versão publicada do catálogo.
type Pacote struct {
	Versao     int64
	SHA256     [32]byte
	Assinatura []byte
	Dados      []byte
}

type Servico struct {
	pool  *pgxpool.Pool
	chave ed25519.PrivateKey

	mu      sync.Mutex
	atual   *Pacote
	revisao int64 // revisão do catálogo que gerou atual
}

func NovoServico(pool *pgxpool.Pool, chave ed25519.PrivateKey) *Servico {
	return &Servico{pool: pool, chave: chave}
}

// ChavePublica é a chave que vai embutida no programa local.
func (s *Servico) ChavePublica() ed25519.PublicKey {
	return s.chave.Public().(ed25519.PublicKey)
}

// Atual devolve o pacote da versão corrente do catálogo, regerando só se o
// catálogo mudou desde a última vez.
func (s *Servico) Atual(ctx context.Context) (*Pacote, error) {
	var revisao int64
	if err := s.pool.QueryRow(ctx, `SELECT revisao FROM catalogo_revisao`).Scan(&revisao); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.atual != nil && s.revisao == revisao {
		return s.atual, nil
	}

	var p *Pacote
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead}, func(tx pgx.Tx) error {
		// Relê a revisão dentro da transação: o pacote corresponde a ela.
		if err := tx.QueryRow(ctx, `SELECT revisao FROM catalogo_revisao`).Scan(&revisao); err != nil {
			return err
		}
		cat, miniaturas, err := lerConteudo(ctx, tx)
		if err != nil {
			return err
		}
		dados, err := montarZIP(cat, miniaturas)
		if err != nil {
			return err
		}
		p, err = s.publicar(ctx, tx, dados)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.atual, s.revisao = p, revisao
	return p, nil
}

// publicar reaproveita a última versão se o conteúdo for o mesmo (e a
// assinatura bater com a chave atual); senão grava uma versão nova.
func (s *Servico) publicar(ctx context.Context, tx pgx.Tx, dados []byte) (*Pacote, error) {
	hash := sha256.Sum256(dados)

	var ultima Pacote
	var hashUltima []byte
	err := tx.QueryRow(ctx,
		`SELECT versao, sha256, assinatura FROM pacotes_catalogo ORDER BY versao DESC LIMIT 1`).
		Scan(&ultima.Versao, &hashUltima, &ultima.Assinatura)
	if err == nil {
		copy(ultima.SHA256[:], hashUltima)
		if ultima.SHA256 == hash && Verificar(s.ChavePublica(), ultima.Versao, hash, ultima.Assinatura) {
			ultima.Dados = dados
			return &ultima, nil
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	p := &Pacote{SHA256: hash, Dados: dados}
	if err := tx.QueryRow(ctx,
		`SELECT nextval(pg_get_serial_sequence('pacotes_catalogo', 'versao'))`).Scan(&p.Versao); err != nil {
		return nil, err
	}
	p.Assinatura = ed25519.Sign(s.chave, Mensagem(p.Versao, hash))
	_, err = tx.Exec(ctx,
		`INSERT INTO pacotes_catalogo (versao, sha256, assinatura, dados) VALUES ($1, $2, $3, $4)`,
		p.Versao, hash[:], p.Assinatura, dados)
	return p, err
}
