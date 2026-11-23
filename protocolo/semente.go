// Package protocolo reúne o que o site, a CDN e o programa local precisam
// combinar entre si: o OT 1-de-n de Naor–Pinkas sobre o ot-helper (D-008), os
// tokens com assinatura cega (RF16, D-011), a cifra dos livros (D-009) e os
// formatos das mensagens (D-012).
package protocolo

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"io"
)

// Semente é o segredo de uma compra, do lado do cliente (RF11, D-001): 256
// bits do gerador do sistema, um por compra, nunca reutilizados. Toda a
// aleatoriedade do cliente na compra (receptor do OT, mensagens dos tokens e
// fatores de cegamento) é derivada dela; por isso o programa local consegue
// refazer tudo a partir da semente e da resposta recebida por e-mail.
type Semente [32]byte

// NovaSemente sorteia uma semente com o gerador criptográfico do sistema.
func NovaSemente() (Semente, error) {
	var s Semente
	_, err := rand.Read(s[:])
	return s, err
}

func (s Semente) derivar(rotulo string, n int) []byte {
	k, err := hkdf.Key(sha256.New, s[:], nil, "livraria-ot/semente/v1/"+rotulo, n)
	if err != nil {
		panic(err) // só falha com tamanho inválido, fixo aqui
	}
	return k
}

// SementeOT é a semente de 128 bits do PRNG do receptor no ot-helper.
func (s Semente) SementeOT() [16]byte {
	var o [16]byte
	copy(o[:], s.derivar("ot", 16))
	return o
}

// Fluxo devolve um gerador determinístico (AES-256-CTR com chave derivada da
// semente) para o rótulo dado. Dois fluxos com rótulos diferentes são
// independentes.
func (s Semente) Fluxo(rotulo string) io.Reader {
	bloco, err := aes.NewCipher(s.derivar("fluxo/"+rotulo, 32))
	if err != nil {
		panic(err)
	}
	var iv [aes.BlockSize]byte
	return &leitorCTR{cipher.NewCTR(bloco, iv[:])}
}

type leitorCTR struct{ s cipher.Stream }

func (l *leitorCTR) Read(p []byte) (int, error) {
	clear(p)
	l.s.XORKeyStream(p, p)
	return len(p), nil
}
