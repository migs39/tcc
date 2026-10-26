package catalogo

import (
	"errors"
	"strings"
)

var ErrISBNInvalido = errors.New("ISBN inválido (use ISBN-10 ou ISBN-13)")

// NormalizarISBN remove hífens e espaços e confere o dígito verificador.
// Vazio é aceito (ISBN é opcional).
func NormalizarISBN(s string) (string, error) {
	s = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))
	switch len(s) {
	case 0:
		return "", nil
	case 10:
		soma := 0
		for i, c := range s {
			var d int
			switch {
			case c >= '0' && c <= '9':
				d = int(c - '0')
			case c == 'X' && i == 9:
				d = 10
			default:
				return "", ErrISBNInvalido
			}
			soma += d * (10 - i)
		}
		if soma%11 != 0 {
			return "", ErrISBNInvalido
		}
	case 13:
		soma := 0
		for i, c := range s {
			if c < '0' || c > '9' {
				return "", ErrISBNInvalido
			}
			peso := 1
			if i%2 == 1 {
				peso = 3
			}
			soma += int(c-'0') * peso
		}
		if soma%10 != 0 {
			return "", ErrISBNInvalido
		}
	default:
		return "", ErrISBNInvalido
	}
	return s, nil
}
