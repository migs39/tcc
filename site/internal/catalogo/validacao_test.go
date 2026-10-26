package catalogo

import "testing"

func TestLerPreco(t *testing.T) {
	validos := map[string]int{
		"29,90": 2990, "29.90": 2990, "R$ 29,90": 2990, "29": 2900, "29,9": 2990, " 0,99 ": 99,
	}
	for entrada, esperado := range validos {
		if v, err := LerPreco(entrada); err != nil || v != esperado {
			t.Errorf("LerPreco(%q) = %d, %v; esperado %d", entrada, v, err, esperado)
		}
	}
	for _, entrada := range []string{"", "abc", "0", "0,00", "-5", "29,999", "29,", ",50", "1.000,00", "+3"} {
		if _, err := LerPreco(entrada); err == nil {
			t.Errorf("LerPreco(%q) aceitou valor inválido", entrada)
		}
	}
}

func TestNormalizarISBN(t *testing.T) {
	validos := map[string]string{
		"":                  "",
		"978-0-306-40615-7": "9780306406157",
		"0-306-40615-2":     "0306406152",
		"0-8044-2957-x":     "080442957X",
	}
	for entrada, esperado := range validos {
		if v, err := NormalizarISBN(entrada); err != nil || v != esperado {
			t.Errorf("NormalizarISBN(%q) = %q, %v; esperado %q", entrada, v, err, esperado)
		}
	}
	for _, entrada := range []string{"9780306406158", "0306406153", "12345", "978030640615X", "X306406152"} {
		if _, err := NormalizarISBN(entrada); err == nil {
			t.Errorf("NormalizarISBN(%q) aceitou ISBN inválido", entrada)
		}
	}
}
