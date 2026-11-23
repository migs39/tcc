package protocolo

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"github.com/cloudflare/circl/blindsign/blindrsa"
)

// Tokens de download com assinatura cega (RF16, RF17, D-011): RSA-3072,
// RFC 9474, variante RSABSSA-SHA384-PSS-Randomized. Uma chave por faixa de
// preço. O site assina às cegas; a CDN verifica com a chave pública, que todos
// recebem no pacote assinado do catálogo (D-007), para o site não poder dar a
// cada cliente uma chave diferente e assim reconhecer os tokens.

// BitsChaveToken atende RNF01 (~128 bits para fatoração).
const BitsChaveToken = 3072

const variante = blindrsa.SHA384PSSRandomized

// Token autoriza um download na CDN.
type Token struct {
	Mensagem   []byte // mensagem preparada (prefixo aleatório + valor do cliente)
	Assinatura []byte
}

// String é a forma enviada à CDN: base64url(mensagem) "." base64url(assinatura).
func (t Token) String() string {
	e := base64.RawURLEncoding
	return e.EncodeToString(t.Mensagem) + "." + e.EncodeToString(t.Assinatura)
}

// LerToken desfaz String.
func LerToken(s string) (Token, error) {
	a, b, ok := strings.Cut(s, ".")
	if !ok {
		return Token{}, errors.New("token malformado")
	}
	e := base64.RawURLEncoding
	m, err1 := e.DecodeString(a)
	sig, err2 := e.DecodeString(b)
	if err1 != nil || err2 != nil || len(m) == 0 || len(m) > 256 || len(sig) == 0 || len(sig) > 1024 {
		return Token{}, errors.New("token malformado")
	}
	return Token{m, sig}, nil
}

// GerarChaveToken cria a chave de uma faixa.
func GerarChaveToken() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, BitsChaveToken) }

// CodificarPublica serializa a chave pública (PKIX DER).
func CodificarPublica(pk *rsa.PublicKey) ([]byte, error) { return x509.MarshalPKIXPublicKey(pk) }

// LerPublica desfaz CodificarPublica e confere o tamanho.
func LerPublica(der []byte) (*rsa.PublicKey, error) {
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	pk, ok := k.(*rsa.PublicKey)
	if !ok || pk.N.BitLen() != BitsChaveToken {
		return nil, errors.New("chave de token inválida")
	}
	return pk, nil
}

// AssinarCego é o lado do site: assina um valor cegado.
func AssinarCego(sk *rsa.PrivateKey, cegado []byte) ([]byte, error) {
	return blindrsa.NewSigner(sk).BlindSign(cegado)
}

// VerificarToken é o lado da CDN.
func VerificarToken(pk *rsa.PublicKey, t Token) error {
	v, err := blindrsa.NewVerifier(variante, pk)
	if err != nil {
		return err
	}
	return v.Verify(t.Mensagem, t.Assinatura)
}

// TokensPendentes é o lado do cliente entre cegar e finalizar.
type TokensPendentes struct {
	cliente    blindrsa.Client
	preparadas [][]byte
	estados    []blindrsa.State
	Cegados    [][]byte // vão para o site
}

// CegarTokens prepara e cega n tokens. Toda a aleatoriedade vem de fluxo
// (derivado da semente da compra), para o programa local poder refazer os
// mesmos estados na recuperação (D-001, D-012).
func CegarTokens(pk *rsa.PublicKey, fluxo io.Reader, n int) (*TokensPendentes, error) {
	c, err := blindrsa.NewClient(variante, pk)
	if err != nil {
		return nil, err
	}
	tp := &TokensPendentes{cliente: c}
	for i := 0; i < n; i++ {
		valor := make([]byte, 32)
		if _, err := io.ReadFull(fluxo, valor); err != nil {
			return nil, err
		}
		prep, err := c.Prepare(fluxo, valor)
		if err != nil {
			return nil, err
		}
		cegado, estado, err := c.Blind(fluxo, prep)
		if err != nil {
			return nil, err
		}
		tp.preparadas = append(tp.preparadas, prep)
		tp.estados = append(tp.estados, estado)
		tp.Cegados = append(tp.Cegados, cegado)
	}
	return tp, nil
}

// Finalizar remove o cegamento das assinaturas e confere cada token.
func (tp *TokensPendentes) Finalizar(assinaturasCegas [][]byte) ([]Token, error) {
	if len(assinaturasCegas) != len(tp.estados) {
		return nil, errors.New("quantidade de assinaturas não confere")
	}
	out := make([]Token, len(tp.estados))
	for i, e := range tp.estados {
		sig, err := tp.cliente.Finalize(e, assinaturasCegas[i])
		if err != nil {
			return nil, err
		}
		out[i] = Token{tp.preparadas[i], sig}
	}
	return out, nil
}
