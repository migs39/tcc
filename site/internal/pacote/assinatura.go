// Package pacote gera e publica o pacote do catálogo (RF04, D-006): um ZIP
// determinístico, igual para todos, assinado com Ed25519.
package pacote

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// prefixoAssinatura separa esta assinatura de qualquer outro uso da chave.
const prefixoAssinatura = "livraria-ot/catalogo/v1\n"

// Mensagem é o que o site assina: prefixo, versão e SHA-256 do ZIP. A versão
// entra na assinatura para o programa local poder recusar pacotes mais antigos
// do que o que já tem.
func Mensagem(versao int64, sha256 [32]byte) []byte {
	return []byte(prefixoAssinatura + strconv.FormatInt(versao, 10) + "\n" + hex.EncodeToString(sha256[:]))
}

// Verificar confere a assinatura de um pacote com a chave pública do site.
func Verificar(publica ed25519.PublicKey, versao int64, sha256 [32]byte, assinatura []byte) bool {
	return len(publica) == ed25519.PublicKeySize && ed25519.Verify(publica, Mensagem(versao, sha256), assinatura)
}

// CarregarOuCriarChave lê a semente Ed25519 (hex) do arquivo; se o arquivo não
// existir, cria uma chave nova com o CSPRNG do sistema e grava. Devolve também
// se a chave foi criada agora.
func CarregarOuCriarChave(caminho string) (ed25519.PrivateKey, bool, error) {
	conteudo, err := os.ReadFile(caminho)
	if errors.Is(err, os.ErrNotExist) {
		semente := make([]byte, ed25519.SeedSize)
		if _, err := rand.Read(semente); err != nil {
			return nil, false, err
		}
		if err := os.WriteFile(caminho, []byte(hex.EncodeToString(semente)+"\n"), 0o600); err != nil {
			return nil, false, fmt.Errorf("gravando chave do pacote: %w", err)
		}
		return ed25519.NewKeyFromSeed(semente), true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("lendo chave do pacote: %w", err)
	}
	semente, err := hex.DecodeString(strings.TrimSpace(string(conteudo)))
	if err != nil || len(semente) != ed25519.SeedSize {
		return nil, false, fmt.Errorf("arquivo de chave do pacote inválido: %s", caminho)
	}
	return ed25519.NewKeyFromSeed(semente), false, nil
}
