package cliente

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"tcc/protocolo"
)

func getenv(k, p string) string { return padrao(os.Getenv(k), p) }

func padrao(v, p string) string {
	if v != "" {
		return strings.TrimRight(v, "/")
	}
	return p
}

// Embutidos são os valores gravados no executável na compilação
// (-ldflags "-X main.chavePacote=... -X main.siteURL=... -X main.cdnURL=...").
// Assim o programa distribuído já sai apontando para o site e a CDN públicos.
type Embutidos struct {
	ChavePacote, SiteURL, CDNURL string
}

// Configurar monta o cliente a partir das variáveis de ambiente (ver o
// comando livraria) e dos valores embutidos na compilação. Para os endereços,
// a variável de ambiente vence o valor embutido, que vence o localhost.
// O programa só fala com esses dois endereços (D-004).
func Configurar(e Embutidos) (*Cliente, error) {
	h := e.ChavePacote
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
		SiteURL:     getenv("SITE_URL", padrao(e.SiteURL, "http://localhost:8080")),
		CDNURL:      getenv("CDN_URL", padrao(e.CDNURL, "http://localhost:8081")),
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
