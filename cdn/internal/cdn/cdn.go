// Package cdn implementa a CDN (RF17, RF18): guarda os livros cifrados,
// entrega um livro por token cego válido e ainda não usado e reporta ao site
// só os tokens usados e a quantidade, em lotes embaralhados e sem horários
// (D-011).
//
// A CDN sabe quais livros foram baixados, por token; não sabe quem comprou
// (os tokens não carregam identidade, RF16). Ela não registra endereço, horário
// nem livro de cada token: a tabela de tokens só tem o resumo do token, o
// próprio token até ser reportado e se já foi reportado.
package cdn

import (
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"tcc/protocolo"
)

// Config da CDN.
type Config struct {
	Pasta         string            // onde ficam os livros cifrados
	SiteURL       string            // de onde vem o pacote do catálogo e para onde vai o reporte
	Segredo       string            // segredo compartilhado com o site
	ChavePacote   ed25519.PublicKey // chave pública do pacote do catálogo (D-007)
	LoteMinimo    int               // envia o reporte ao juntar este número de tokens
	IntervaloLote time.Duration     // ou quando passar este tempo desde o último lote
	HTTP          *http.Client
}

type CDN struct {
	cfg  Config
	pool *pgxpool.Pool

	mu         sync.RWMutex
	faixaLivro map[int64]int64
	chaves     map[int64]*rsa.PublicKey
	versaoCat  int64
	ultimoLote time.Time
	enviouLote bool
}

func Nova(pool *pgxpool.Pool, cfg Config) *CDN {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.LoteMinimo <= 0 {
		cfg.LoteMinimo = 50
	}
	if cfg.IntervaloLote <= 0 {
		cfg.IntervaloLote = 24 * time.Hour
	}
	return &CDN{cfg: cfg, pool: pool, ultimoLote: time.Now()}
}

// Migrar cria as tabelas da CDN no schema atual.
func Migrar(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS tokens_usados (
		    resumo     BYTEA   PRIMARY KEY CHECK (octet_length(resumo) = 32),
		    token      TEXT,
		    reportado  BOOLEAN NOT NULL DEFAULT FALSE
		)`)
	return err
}

// AtualizarCatalogo baixa e confere o pacote do catálogo do site para saber a
// faixa de cada livro e a chave pública de token de cada faixa.
func (c *CDN) AtualizarCatalogo(ctx context.Context) error {
	c.mu.RLock()
	minima := c.versaoCat
	c.mu.RUnlock()
	pb, err := protocolo.BaixarPacote(ctx, c.cfg.HTTP, c.cfg.SiteURL, c.cfg.ChavePacote, minima)
	if err != nil {
		return err
	}
	cat, versao := pb.Catalogo, pb.Versao
	faixaLivro := map[int64]int64{}
	for _, l := range cat.Livros {
		faixaLivro[l.ID] = l.FaixaID
	}
	ch := map[int64]*rsa.PublicKey{}
	for _, g := range cat.Grupos {
		if len(g.ChaveToken) == 0 {
			continue
		}
		pk, err := protocolo.LerPublica(g.ChaveToken)
		if err != nil {
			return err
		}
		ch[g.FaixaID] = pk
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if versao < c.versaoCat {
		return errors.New("catálogo mais antigo que o atual")
	}
	c.faixaLivro, c.chaves, c.versaoCat = faixaLivro, ch, versao
	return nil
}

func (c *CDN) caminho(id int64) string {
	return filepath.Join(c.cfg.Pasta, strconv.FormatInt(id, 10)+".bin")
}

// Guardar grava o livro cifrado enviado pelo site.
func (c *CDN) Guardar(id int64, cifrado []byte) error {
	if err := os.MkdirAll(c.cfg.Pasta, 0o700); err != nil {
		return err
	}
	tmp := c.caminho(id) + ".tmp"
	if err := os.WriteFile(tmp, cifrado, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.caminho(id))
}

var (
	ErrLivroInexistente = errors.New("livro não disponível")
	ErrTokenInvalido    = errors.New("token inválido")
	ErrTokenUsado       = errors.New("token já usado")
)

// Entregar valida o token para a faixa do livro, marca-o como usado (uso
// único, RF17, D-011) e devolve o livro cifrado.
func (c *CDN) Entregar(ctx context.Context, id int64, tokenStr string) ([]byte, error) {
	dados, err := os.ReadFile(c.caminho(id))
	if err != nil {
		return nil, ErrLivroInexistente
	}
	c.mu.RLock()
	faixa, ok := c.faixaLivro[id]
	pk := c.chaves[faixa]
	c.mu.RUnlock()
	if !ok || pk == nil {
		// Livro novo no catálogo: tenta atualizar uma vez.
		if err := c.AtualizarCatalogo(ctx); err == nil {
			c.mu.RLock()
			faixa, ok = c.faixaLivro[id]
			pk = c.chaves[faixa]
			c.mu.RUnlock()
		}
		if !ok || pk == nil {
			return nil, ErrLivroInexistente
		}
	}
	tok, err := protocolo.LerToken(tokenStr)
	if err != nil || protocolo.VerificarToken(pk, tok) != nil {
		return nil, ErrTokenInvalido
	}
	resumo := sha256.Sum256(tok.Mensagem)
	tag, err := c.pool.Exec(ctx,
		`INSERT INTO tokens_usados (resumo, token) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		resumo[:], tok.String())
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrTokenUsado
	}
	return dados, nil
}
