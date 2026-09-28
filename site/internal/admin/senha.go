package admin

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parâmetros do argon2id: segunda recomendação da RFC 9106 (seção 4),
// 64 MiB de memória, 3 passadas, 4 vias; sal de 128 bits e saída de 256 bits.
const (
	argonMemoriaKiB = 64 * 1024
	argonPassadas   = 3
	argonVias       = 4
	argonTamSal     = 16
	argonTamHash    = 32
)

// TamanhoMinimoSenha é o mínimo aceito para senhas de administrador (RF01).
const TamanhoMinimoSenha = 12

var ErrHashInvalido = errors.New("hash de senha em formato inválido")

// GerarHashSenha devolve o hash argon2id no formato PHC:
// $argon2id$v=19$m=65536,t=3,p=4$<sal>$<hash>
func GerarHashSenha(senha string) (string, error) {
	sal := make([]byte, argonTamSal)
	if _, err := rand.Read(sal); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(senha), sal, argonPassadas, argonMemoriaKiB, argonVias, argonTamHash)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoriaKiB, argonPassadas, argonVias,
		base64.RawStdEncoding.EncodeToString(sal),
		base64.RawStdEncoding.EncodeToString(hash)), nil
}

// VerificarSenha compara a senha com o hash em tempo constante. Usa os
// parâmetros gravados no próprio hash, para que hashes antigos continuem
// válidos se os parâmetros mudarem.
func VerificarSenha(senha, hashPHC string) (bool, error) {
	partes := strings.Split(hashPHC, "$")
	if len(partes) != 6 || partes[1] != "argon2id" {
		return false, ErrHashInvalido
	}
	var versao int
	if _, err := fmt.Sscanf(partes[2], "v=%d", &versao); err != nil || versao != argon2.Version {
		return false, ErrHashInvalido
	}
	var memoria, passadas uint32
	var vias uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &memoria, &passadas, &vias); err != nil {
		return false, ErrHashInvalido
	}
	sal, err := base64.RawStdEncoding.DecodeString(partes[4])
	if err != nil {
		return false, ErrHashInvalido
	}
	esperado, err := base64.RawStdEncoding.DecodeString(partes[5])
	if err != nil || len(esperado) == 0 {
		return false, ErrHashInvalido
	}
	calculado := argon2.IDKey([]byte(senha), sal, passadas, memoria, vias, uint32(len(esperado)))
	return subtle.ConstantTimeCompare(calculado, esperado) == 1, nil
}
