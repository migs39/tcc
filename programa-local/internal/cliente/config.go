package cliente

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"tcc/protocolo"
)

func getenv(k, padrao string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return padrao
}

// Configurar monta o cliente a partir das variáveis de ambiente (ver o
// comando livraria) e da chave pública do pacote embutida na compilação.
func Configurar(chaveEmbutida string) (*Cliente, error) {
	h := chaveEmbutida
	if h == "" {
		h = os.Getenv("CHAVE_PACOTE_PUB")
	}
	pub, err := hex.DecodeString(h)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("chave pública do catálogo ausente: compile com -ldflags \"-X main.chavePacote=...\" ou defina CHAVE_PACOTE_PUB")
	}
	pasta := os.Getenv("LIVRARIA_DADOS")
	if pasta == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		pasta = filepath.Join(base, "livraria-ot")
	}
	return &Cliente{
		Pasta:       pasta,
		SiteURL:     getenv("SITE_URL", "http://localhost:8080"),
		CDNURL:      getenv("CDN_URL", "http://localhost:8081"),
		ChavePacote: ed25519.PublicKey(pub),
		OT:          protocolo.OTHelper{Caminho: caminhoOT()},
	}, nil
}

// caminhoOT procura o ot-helper ao lado do executável antes do PATH.
func caminhoOT() string {
	if c := os.Getenv("OT_HELPER"); c != "" {
		return c
	}
	nome := "ot-helper"
	if runtime.GOOS == "windows" {
		nome += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		c := filepath.Join(filepath.Dir(exe), nome)
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if c, err := exec.LookPath(nome); err == nil {
		return c
	}
	return nome
}
