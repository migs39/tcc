package catalogo

import (
	"errors"
	"strconv"
	"strings"
)

var ErrPrecoInvalido = errors.New("preço inválido: use o formato 29,90")

// LerPreco converte "29,90", "29.90", "R$ 29,90" ou "29" em centavos.
func LerPreco(s string) (int, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "R$"))
	if s == "" {
		return 0, ErrPrecoInvalido
	}
	inteiro, frac, temFrac := strings.Cut(strings.ReplaceAll(s, ",", "."), ".")
	if temFrac && (len(frac) == 0 || len(frac) > 2) {
		return 0, ErrPrecoInvalido
	}
	for len(frac) < 2 {
		frac += "0"
	}
	if inteiro == "" || strings.ContainsAny(inteiro+frac, "+-") {
		return 0, ErrPrecoInvalido
	}
	reais, err := strconv.Atoi(inteiro)
	if err != nil || reais > 1_000_000 {
		return 0, ErrPrecoInvalido
	}
	cent, err := strconv.Atoi(frac)
	if err != nil {
		return 0, ErrPrecoInvalido
	}
	total := reais*100 + cent
	if total <= 0 {
		return 0, ErrPrecoInvalido
	}
	return total, nil
}
