package protocolo

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Leitura do pacote do catálogo do lado de quem o consome (programa local e
// CDN). O formato é o da D-007; o site o gera em site/internal/pacote.

type Catalogo struct {
	Formato    int         `json:"formato"`
	Categorias []Categoria `json:"categorias"`
	Grupos     []Grupo     `json:"grupos"`
	Livros     []Livro     `json:"livros"`
}

type Categoria struct {
	ID   int64  `json:"id"`
	Nome string `json:"nome"`
}

// Grupo é a versão atual de um grupo; o índice no OT é a posição em Livros.
type Grupo struct {
	FaixaID       int64   `json:"faixa_id"`
	PrecoCentavos int     `json:"preco_centavos"`
	VersaoGrupoID int64   `json:"versao_grupo_id"`
	VersaoNumero  int     `json:"versao_numero"`
	Livros        []int64 `json:"livros"`
	ChaveToken    []byte  `json:"chave_token,omitempty"` // PKIX DER (D-011)
}

type Livro struct {
	ID            int64  `json:"id"`
	Titulo        string `json:"titulo"`
	Autor         string `json:"autor"`
	CategoriaID   int64  `json:"categoria_id"`
	Descricao     string `json:"descricao"`
	Ano           int    `json:"ano,omitempty"`
	ISBN          string `json:"isbn,omitempty"`
	Editora       string `json:"editora,omitempty"`
	FaixaID       int64  `json:"faixa_id"`
	PrecoCentavos int    `json:"preco_centavos"`
	Miniatura     string `json:"miniatura,omitempty"`
}

// MensagemPacote é o que a assinatura Ed25519 do pacote cobre (D-007).
func MensagemPacote(versao int64, sha [32]byte) []byte {
	return []byte("livraria-ot/catalogo/v1\n" + strconv.FormatInt(versao, 10) + "\n" + hex.EncodeToString(sha[:]))
}

// AbrirPacote confere a assinatura e o resumo e lê o catalogo.json. As
// miniaturas ficam no ZIP devolvido.
func AbrirPacote(publica ed25519.PublicKey, versao int64, assinatura, dados []byte) (Catalogo, *zip.Reader, error) {
	sha := sha256.Sum256(dados)
	if !ed25519.Verify(publica, MensagemPacote(versao, sha), assinatura) {
		return Catalogo{}, nil, errors.New("assinatura do pacote do catálogo inválida")
	}
	zr, err := zip.NewReader(bytes.NewReader(dados), int64(len(dados)))
	if err != nil {
		return Catalogo{}, nil, err
	}
	f, err := zr.Open("catalogo.json")
	if err != nil {
		return Catalogo{}, nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 64<<20))
	if err != nil {
		return Catalogo{}, nil, err
	}
	var c Catalogo
	if err := json.Unmarshal(b, &c); err != nil {
		return Catalogo{}, nil, err
	}
	if c.Formato != 1 {
		return Catalogo{}, nil, errors.New("formato de catálogo desconhecido")
	}
	return c, zr, nil
}

// GrupoDaVersao procura o grupo pela versão.
func (c Catalogo) GrupoDaVersao(versao int64) (Grupo, bool) {
	for _, g := range c.Grupos {
		if g.VersaoGrupoID == versao {
			return g, true
		}
	}
	return Grupo{}, false
}

// PacoteBaixado é um pacote do catálogo já conferido.
type PacoteBaixado struct {
	Versao     int64
	Assinatura []byte
	Dados      []byte
	Catalogo   Catalogo
	ZIP        *zip.Reader
}

// BaixarPacote busca /catalogo.zip, confere assinatura e resumo com a chave
// pública e recusa versão menor que versaoMinima (contra rollback, D-007).
// Todos os clientes fazem exatamente a mesma requisição.
func BaixarPacote(ctx context.Context, cli *http.Client, siteURL string, publica ed25519.PublicKey, versaoMinima int64) (*PacoteBaixado, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(siteURL, "/")+"/catalogo.zip", nil)
	if err != nil {
		return nil, err
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("site devolveu %d para o catálogo", resp.StatusCode)
	}
	dados, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return nil, err
	}
	versao, err := strconv.ParseInt(resp.Header.Get("Catalogo-Versao"), 10, 64)
	if err != nil {
		return nil, errors.New("catálogo sem versão")
	}
	if versao < versaoMinima {
		return nil, fmt.Errorf("catálogo na versão %d, mais antiga que a %d já conhecida", versao, versaoMinima)
	}
	assin, err := base64.StdEncoding.DecodeString(resp.Header.Get("Catalogo-Assinatura"))
	if err != nil {
		return nil, errors.New("assinatura do catálogo malformada")
	}
	cat, zr, err := AbrirPacote(publica, versao, assin, dados)
	if err != nil {
		return nil, err
	}
	return &PacoteBaixado{Versao: versao, Assinatura: assin, Dados: dados, Catalogo: cat, ZIP: zr}, nil
}
