package cdn

import (
	"crypto/subtle"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// TamanhoMaximoLivro limita o upload do site.
const TamanhoMaximoLivro = 256 << 20

// Rotas monta as rotas da CDN. Não há log de acesso: nada de endereço, horário
// ou livro por requisição.
func (c *CDN) Rotas(log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livros/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			http.NotFound(w, r)
			return
		}
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Token ")
		if !ok {
			http.Error(w, "token ausente", http.StatusUnauthorized)
			return
		}
		dados, err := c.Entregar(r.Context(), id, tok)
		switch {
		case errors.Is(err, ErrLivroInexistente):
			http.NotFound(w, r)
		case errors.Is(err, ErrTokenInvalido):
			http.Error(w, "token inválido", http.StatusForbidden)
		case errors.Is(err, ErrTokenUsado):
			http.Error(w, "token já usado", http.StatusForbidden)
		case err != nil:
			log.Error("erro interno na entrega", "erro", err)
			http.Error(w, "erro interno", http.StatusInternalServerError)
		default:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Length", strconv.Itoa(len(dados)))
			w.Write(dados)
		}
	})
	mux.HandleFunc("PUT /admin/livros/{id}", func(w http.ResponseWriter, r *http.Request) {
		esperado := "Bearer " + c.cfg.Segredo
		if c.cfg.Segredo == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(esperado)) != 1 {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			http.NotFound(w, r)
			return
		}
		dados, err := io.ReadAll(http.MaxBytesReader(w, r.Body, TamanhoMaximoLivro))
		if err != nil || len(dados) == 0 {
			http.Error(w, "arquivo inválido", http.StatusBadRequest)
			return
		}
		if err := c.Guardar(id, dados); err != nil {
			log.Error("gravando livro cifrado", "erro", err)
			http.Error(w, "erro interno", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	c.rotasPainel(mux)
	return mux
}
