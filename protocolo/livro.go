package protocolo

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"strconv"
)

// Cifra dos livros (D-009): AES-256-GCM, chave aleatória por livro, nonce de
// 12 bytes na frente, ID do livro como dado associado (um arquivo cifrado não
// passa por outro livro).

func dadoAssociado(livroID int64) []byte {
	return []byte("livraria-ot/livro/v1/" + strconv.FormatInt(livroID, 10))
}

// NovaChaveLivro sorteia uma chave.
func NovaChaveLivro() ([TamChave]byte, error) {
	var k [TamChave]byte
	_, err := rand.Read(k[:])
	return k, err
}

// CifrarLivro devolve nonce ‖ cifra+tag.
func CifrarLivro(chave [TamChave]byte, livroID int64, dados []byte) ([]byte, error) {
	b, err := aes.NewCipher(chave[:])
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize(), g.NonceSize()+len(dados)+g.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return g.Seal(nonce, nonce, dados, dadoAssociado(livroID)), nil
}

// DecifrarLivro desfaz CifrarLivro; falha com chave errada (RF12).
func DecifrarLivro(chave [TamChave]byte, livroID int64, cifrado []byte) ([]byte, error) {
	b, err := aes.NewCipher(chave[:])
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	if len(cifrado) < g.NonceSize()+g.Overhead() {
		return nil, errors.New("livro cifrado curto demais")
	}
	return g.Open(nil, cifrado[:g.NonceSize()], cifrado[g.NonceSize():], dadoAssociado(livroID))
}
