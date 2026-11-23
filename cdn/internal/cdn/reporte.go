package cdn

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tcc/protocolo"
)

// Reportar envia ao site o lote de tokens usados e ainda não reportados, se
// houver pelo menos LoteMinimo ou se já passou IntervaloLote desde o último
// (D-011). O lote vai embaralhado, sem horários e sem livro (RF18). Devolve
// quantos tokens foram reportados.
func (c *CDN) Reportar(ctx context.Context, forcar bool) (int, error) {
	linhas, err := c.pool.Query(ctx, `SELECT token FROM tokens_usados WHERE NOT reportado`)
	if err != nil {
		return 0, err
	}
	tokens, err := pgx.CollectRows(linhas, pgx.RowTo[string])
	if err != nil || len(tokens) == 0 {
		return 0, err
	}
	c.mu.RLock()
	venceu := time.Since(c.ultimoLote) >= c.cfg.IntervaloLote
	c.mu.RUnlock()
	if !forcar && len(tokens) < c.cfg.LoteMinimo && !venceu {
		return 0, nil
	}
	embaralhar(tokens)

	corpo, _ := json.Marshal(protocolo.ReporteCDN{Tokens: tokens, Quantidade: len(tokens)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.cfg.SiteURL, "/")+"/api/cdn/reporte", bytes.NewReader(corpo))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Segredo)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("site recusou o lote (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	// Reportado: o token em si não é mais necessário; fica só o resumo para
	// recusar reuso.
	if _, err := c.pool.Exec(ctx,
		`UPDATE tokens_usados SET reportado = TRUE, token = NULL WHERE token = ANY($1)`, tokens); err != nil {
		return 0, err
	}
	c.mu.Lock()
	c.ultimoLote, c.enviouLote = time.Now(), true
	c.mu.Unlock()
	return len(tokens), nil
}

// embaralhar usa Fisher–Yates com o gerador do sistema: a ordem do lote não
// conta a ordem dos downloads.
func embaralhar(s []string) {
	for i := len(s) - 1; i > 0; i-- {
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		k := int(j.Int64())
		s[i], s[k] = s[k], s[i]
	}
}

// RodarReportes verifica a cada minuto se é hora de mandar um lote.
func (c *CDN) RodarReportes(ctx context.Context, erro func(error)) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := c.Reportar(ctx, false); err != nil {
				erro(err)
			}
		}
	}
}
