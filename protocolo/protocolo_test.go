package protocolo

import (
	"bytes"
	"context"
	"crypto/rsa"
	"os"
	"os/exec"
	"testing"
)

// helper devolve o ot-helper (OT_HELPER ou PATH); pula o teste sem ele.
func helper(t *testing.T) OTHelper {
	t.Helper()
	c := os.Getenv("OT_HELPER")
	if c == "" {
		var err error
		if c, err = exec.LookPath("ot-helper"); err != nil {
			t.Skip("ot-helper indisponível; defina OT_HELPER")
		}
	}
	return OTHelper{Caminho: c}
}

func TestBitsIndice(t *testing.T) {
	casos := map[int]int{1: 1, 2: 1, 3: 2, 4: 2, 5: 3, 8: 3, 9: 4, 1000: 10, 1024: 10, 1025: 11}
	for n, l := range casos {
		if BitsIndice(n) != l {
			t.Errorf("BitsIndice(%d) = %d, quero %d", n, BitsIndice(n), l)
		}
	}
}

// RF07, RF08, RF12, D-008: o cliente recupera exatamente as chaves escolhidas
// e nenhuma outra posição abre com o que ele recebeu.
func TestNaorPinkasComOTHelper(t *testing.T) {
	ot := helper(t)
	ctx := context.Background()
	const n = 7
	chaves := make([][TamChave]byte, n)
	for i := range chaves {
		chaves[i], _ = NovaChaveLivro()
	}
	indices := []int{6, 0, 3}
	escolhas, err := EscolhasOT(indices, n)
	if err != nil {
		t.Fatal(err)
	}
	sem, _ := NovaSemente()

	req, err := ot.Requisicao(ctx, sem.SementeOT(), escolhas)
	if err != nil {
		t.Fatal(err)
	}
	resp, pares, err := ot.Sender(ctx, req, len(escolhas))
	if err != nil {
		t.Fatal(err)
	}
	cifras, err := CifrarGrupo(chaves, pares, len(indices))
	if err != nil {
		t.Fatal(err)
	}
	recebidos, err := ot.Extrair(ctx, sem.SementeOT(), escolhas, resp)
	if err != nil {
		t.Fatal(err)
	}
	abertas, err := AbrirGrupo(cifras, recebidos, indices, n)
	if err != nil {
		t.Fatal(err)
	}
	for k, i := range indices {
		if abertas[k] != chaves[i] {
			t.Errorf("instância %d: chave do livro %d não confere", k, i)
		}
	}
	// Com os mesmos blocos, qualquer outro índice dá lixo (RF12).
	for i := 0; i < n; i++ {
		if i == indices[0] {
			continue
		}
		outra, _ := AbrirGrupo(cifras[:1], recebidos[:BitsIndice(n)], []int{i}, n)
		if outra[0] == chaves[i] {
			t.Errorf("índice %d não escolhido abriu", i)
		}
	}
}

// D-001: a mesma semente reproduz a requisição byte a byte; outra semente não.
func TestRequisicaoDeterministica(t *testing.T) {
	ot := helper(t)
	ctx := context.Background()
	sem, _ := NovaSemente()
	esc, _ := EscolhasOT([]int{2, 4}, 5)
	a, err := ot.Requisicao(ctx, sem.SementeOT(), esc)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ot.Requisicao(ctx, sem.SementeOT(), esc)
	if !bytes.Equal(a, b) {
		t.Fatal("mesma semente gerou requisições diferentes")
	}
	outra, _ := NovaSemente()
	c, _ := ot.Requisicao(ctx, outra.SementeOT(), esc)
	if bytes.Equal(a, c) {
		t.Fatal("sementes diferentes geraram a mesma requisição")
	}
}

// Invariante 1: a requisição não depende dos índices escolhidos de forma
// visível no tamanho.
func TestRequisicaoNaoVariaDeTamanhoComEscolha(t *testing.T) {
	ot := helper(t)
	ctx := context.Background()
	sem, _ := NovaSemente()
	tam := -1
	for i := 0; i < 8; i++ {
		esc, _ := EscolhasOT([]int{i}, 8)
		r, err := ot.Requisicao(ctx, sem.SementeOT(), esc)
		if err != nil {
			t.Fatal(err)
		}
		if tam >= 0 && len(r) != tam {
			t.Fatal("tamanho da requisição depende da escolha")
		}
		tam = len(r)
	}
}

var chaveTeste *rsa.PrivateKey

func chave(t *testing.T) *rsa.PrivateKey {
	if chaveTeste == nil {
		var err error
		if chaveTeste, err = GerarChaveToken(); err != nil {
			t.Fatal(err)
		}
	}
	return chaveTeste
}

// RF16, RF17, D-012: tokens válidos; a mesma semente refaz os mesmos cegados.
func TestTokens(t *testing.T) {
	sk := chave(t)
	der, _ := CodificarPublica(&sk.PublicKey)
	pk, err := LerPublica(der)
	if err != nil {
		t.Fatal(err)
	}
	sem, _ := NovaSemente()
	tp, err := CegarTokens(pk, sem.Fluxo("tokens"), 3)
	if err != nil {
		t.Fatal(err)
	}
	tp2, _ := CegarTokens(pk, sem.Fluxo("tokens"), 3)
	for i := range tp.Cegados {
		if !bytes.Equal(tp.Cegados[i], tp2.Cegados[i]) {
			t.Fatal("cegamento não é reproduzível a partir da semente")
		}
	}
	var sigs [][]byte
	for _, c := range tp.Cegados {
		s, err := AssinarCego(sk, c)
		if err != nil {
			t.Fatal(err)
		}
		sigs = append(sigs, s)
	}
	toks, err := tp2.Finalizar(sigs)
	if err != nil {
		t.Fatal(err)
	}
	for i, tk := range toks {
		lido, err := LerToken(tk.String())
		if err != nil {
			t.Fatal(err)
		}
		if err := VerificarToken(pk, lido); err != nil {
			t.Fatalf("token %d inválido: %v", i, err)
		}
		// RF16: o que o site viu (cegado, assinatura cega) não aparece no token.
		if bytes.Equal(tk.Assinatura, sigs[i]) || bytes.Contains(tp.Cegados[i], tk.Mensagem) {
			t.Fatal("token igual ao que o site viu")
		}
	}
	adulterado := toks[0]
	adulterado.Mensagem = append([]byte{}, adulterado.Mensagem...)
	adulterado.Mensagem[0] ^= 1
	if VerificarToken(pk, adulterado) == nil {
		t.Fatal("token adulterado aceito")
	}
}

// D-009, RF12.
func TestLivro(t *testing.T) {
	k, _ := NovaChaveLivro()
	c, err := CifrarLivro(k, 7, []byte("conteúdo"))
	if err != nil {
		t.Fatal(err)
	}
	if p, err := DecifrarLivro(k, 7, c); err != nil || string(p) != "conteúdo" {
		t.Fatal("ida e volta falhou")
	}
	if _, err := DecifrarLivro(k, 8, c); err == nil {
		t.Fatal("arquivo aceito como outro livro")
	}
	outra, _ := NovaChaveLivro()
	if _, err := DecifrarLivro(outra, 7, c); err == nil {
		t.Fatal("chave errada aceita")
	}
}
