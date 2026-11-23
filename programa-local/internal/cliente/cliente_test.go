package cliente

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// RF17: livro já baixado cujo arquivo sumiu da biblioteca é apontado como
// ausente (não há segundo download); pendente ou presente, não.
func TestAusentes(t *testing.T) {
	dir := t.TempDir()
	presente := filepath.Join(dir, "1 - A.txt")
	if err := os.WriteFile(presente, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	cp := &Compra{
		Titulos:  []string{"A", "B", "C"},
		Baixados: []string{presente, filepath.Join(dir, "2 - B.txt"), ""},
	}
	if got := ausentes(cp); !reflect.DeepEqual(got, []string{"B"}) {
		t.Fatalf("ausentes = %v, esperado [B]", got)
	}
}
