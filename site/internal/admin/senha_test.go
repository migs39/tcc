package admin

import (
	"strings"
	"testing"
)

// RF01: senhas guardadas só como argon2id, com sal aleatório.
func TestHashSenhaVerifica(t *testing.T) {
	h, err := GerarHashSenha("uma senha bem comprida")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("formato inesperado: %s", h)
	}
	if strings.Contains(h, "uma senha bem comprida") {
		t.Fatal("hash contém a senha")
	}
	ok, err := VerificarSenha("uma senha bem comprida", h)
	if err != nil || !ok {
		t.Fatalf("senha correta rejeitada: ok=%v err=%v", ok, err)
	}
	ok, err = VerificarSenha("uma senha bem comprid", h)
	if err != nil || ok {
		t.Fatalf("senha errada aceita: ok=%v err=%v", ok, err)
	}
}

func TestHashSenhaUsaSalDiferente(t *testing.T) {
	a, _ := GerarHashSenha("mesma senha para os dois")
	b, _ := GerarHashSenha("mesma senha para os dois")
	if a == b {
		t.Fatal("dois hashes da mesma senha são iguais: sal não é aleatório")
	}
}

func TestVerificarSenhaRejeitaHashMalformado(t *testing.T) {
	for _, h := range []string{
		"",
		"texto qualquer",
		"$argon2i$v=19$m=65536,t=3,p=4$c2Fs$aGFzaA",
		"$argon2id$v=18$m=65536,t=3,p=4$c2Fs$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=4$@@$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=4$c2Fs$",
	} {
		if _, err := VerificarSenha("x", h); err == nil {
			t.Errorf("hash malformado aceito: %q", h)
		}
	}
}
