// Package chaves guarda as chaves do site: a AES-256 de cada livro (D-009) e
// a RSA de assinatura cega de cada faixa de preço (D-011). Nada daqui vai para
// log, mensagem de erro ou página.
package chaves

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"tcc/protocolo"
)

// Consulta é o que este pacote usa de um pool ou de uma transação.
type Consulta interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// GarantirChavesToken cria a chave de cada faixa ativa que ainda não tem uma.
// A geração RSA-3072 é lenta, por isso fica fora de transações longas.
func GarantirChavesToken(ctx context.Context, pool *pgxpool.Pool) error {
	linhas, err := pool.Query(ctx,
		`SELECT f.id FROM faixas_preco f LEFT JOIN chaves_tokens c ON c.faixa_id = f.id
		  WHERE f.ativa AND c.faixa_id IS NULL ORDER BY f.id`)
	if err != nil {
		return err
	}
	faltando, err := pgx.CollectRows(linhas, pgx.RowTo[int64])
	if err != nil {
		return err
	}
	for _, id := range faltando {
		sk, err := protocolo.GerarChaveToken()
		if err != nil {
			return err
		}
		priv, err := x509.MarshalPKCS8PrivateKey(sk)
		if err != nil {
			return err
		}
		pub, err := protocolo.CodificarPublica(&sk.PublicKey)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO chaves_tokens (faixa_id, privada, publica) VALUES ($1, $2, $3)
			 ON CONFLICT (faixa_id) DO NOTHING`, id, priv, pub); err != nil {
			return err
		}
	}
	return nil
}

// ErrSemChaveToken indica faixa sem chave de token.
var ErrSemChaveToken = errors.New("faixa sem chave de token")

// ChaveToken devolve a chave privada da faixa.
func ChaveToken(ctx context.Context, q Consulta, faixaID int64) (*rsa.PrivateKey, error) {
	var der []byte
	err := q.QueryRow(ctx, `SELECT privada FROM chaves_tokens WHERE faixa_id = $1`, faixaID).Scan(&der)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSemChaveToken
	}
	if err != nil {
		return nil, err
	}
	k, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	sk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("chave de token corrompida")
	}
	return sk, nil
}

// ChavesPublicasToken devolve a chave pública de cada faixa (para validar o
// reporte da CDN).
func ChavesPublicasToken(ctx context.Context, pool *pgxpool.Pool) (map[int64]*rsa.PublicKey, error) {
	linhas, err := pool.Query(ctx, `SELECT faixa_id, publica FROM chaves_tokens ORDER BY faixa_id`)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	out := map[int64]*rsa.PublicKey{}
	for linhas.Next() {
		var id int64
		var der []byte
		if err := linhas.Scan(&id, &der); err != nil {
			return nil, err
		}
		pk, err := protocolo.LerPublica(der)
		if err != nil {
			return nil, err
		}
		out[id] = pk
	}
	return out, linhas.Err()
}

// ChavesLivros devolve a chave de cada livro, na ordem dada, criando as que
// faltam. Uma chave nasce uma vez e não muda: o livro cifrado na CDN depende
// dela.
func ChavesLivros(ctx context.Context, q Consulta, livros []int64) ([][protocolo.TamChave]byte, error) {
	out := make([][protocolo.TamChave]byte, len(livros))
	for i, id := range livros {
		k, err := protocolo.NovaChaveLivro()
		if err != nil {
			return nil, err
		}
		if _, err := q.Exec(ctx,
			`INSERT INTO chaves_livros (livro_id, chave) VALUES ($1, $2) ON CONFLICT (livro_id) DO NOTHING`,
			id, k[:]); err != nil {
			return nil, err
		}
		var b []byte
		if err := q.QueryRow(ctx, `SELECT chave FROM chaves_livros WHERE livro_id = $1`, id).Scan(&b); err != nil {
			return nil, err
		}
		copy(out[i][:], b)
	}
	return out, nil
}
