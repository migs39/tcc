package compra

import (
	"bytes"
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ClienteCDN envia livros cifrados à CDN. Site e CDN se autenticam com um
// segredo compartilhado (SEGREDO_CDN), nos dois sentidos.
type ClienteCDN struct {
	URL     string
	Segredo string
	HTTP    *http.Client
}

func (c *ClienteCDN) Publicar(ctx context.Context, livroID int64, cifrado []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		strings.TrimRight(c.URL, "/")+"/admin/livros/"+strconv.FormatInt(livroID, 10), bytes.NewReader(cifrado))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Segredo)
	req.Header.Set("Content-Type", "application/octet-stream")
	cli := c.HTTP
	if cli == nil {
		cli = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("CDN indisponível: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("CDN recusou o livro (%d)", resp.StatusCode)
	}
	return nil
}

// AutorizadoPorSegredo confere "Authorization: Bearer <segredo>" em tempo
// constante.
func AutorizadoPorSegredo(r *http.Request, segredo string) bool {
	if segredo == "" {
		return false
	}
	h := r.Header.Get("Authorization")
	esperado := "Bearer " + segredo
	return subtle.ConstantTimeCompare([]byte(h), []byte(esperado)) == 1
}
