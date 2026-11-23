package protocolo

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/bits"
)

// OT 1-de-n de Naor e Pinkas (1999) sobre OTs 1-de-2 aleatórios (D-008).
//
// Para um grupo de n livros, cada instância usa ℓ = BitsIndice(n) OTs 1-de-2
// aleatórios: o sender conhece os pares (K_j^0, K_j^1) e o receptor só
// K_j^{b_j}, onde b_1..b_ℓ são os bits do índice escolhido (do mais
// significativo para o menos). A chave do livro i vai cifrada com
//
//	pad_i = SHA-256(rótulo ‖ instância ‖ i ‖ K_1^{i_1} ‖ … ‖ K_ℓ^{i_ℓ})
//
// e só a posição escolhida pode ser aberta. A derivação por SHA-256 é premissa
// de oráculo aleatório (escolha de engenharia, ver D-008).

// TamChave é o tamanho da chave de um livro (AES-256, D-009).
const TamChave = 32

// Bloco é a saída de um OT 1-de-2 (128 bits).
type Bloco = [16]byte

const rotuloNP = "livraria-ot/np/v1"

// BitsIndice é ℓ = ⌈log₂ n⌉ (pelo menos 1).
func BitsIndice(n int) int {
	if n <= 2 {
		return 1
	}
	return bits.Len(uint(n - 1))
}

func pad(instancia, indice int, ks []Bloco) [TamChave]byte {
	h := sha256.New()
	h.Write([]byte(rotuloNP))
	var b [8]byte
	binary.BigEndian.PutUint32(b[:4], uint32(instancia))
	binary.BigEndian.PutUint32(b[4:], uint32(indice))
	h.Write(b[:])
	for _, k := range ks {
		h.Write(k[:])
	}
	var o [TamChave]byte
	copy(o[:], h.Sum(nil))
	return o
}

// bitsDe devolve os ℓ bits do índice, do mais significativo para o menos.
func bitsDe(indice, l int) []bool {
	out := make([]bool, l)
	for j := 0; j < l; j++ {
		out[j] = indice>>(l-1-j)&1 == 1
	}
	return out
}

// EscolhasOT monta a cadeia de bits ('0'/'1') para o ot-helper: ℓ bits por
// instância, na ordem das instâncias.
func EscolhasOT(indices []int, n int) (string, error) {
	l := BitsIndice(n)
	out := make([]byte, 0, len(indices)*l)
	for _, i := range indices {
		if i < 0 || i >= n {
			return "", errors.New("índice fora do grupo")
		}
		for _, b := range bitsDe(i, l) {
			if b {
				out = append(out, '1')
			} else {
				out = append(out, '0')
			}
		}
	}
	return string(out), nil
}

// CifrarGrupo é o lado do sender: para cada instância, cifra as n chaves dos
// livros com os pares dos seus ℓ OTs. pares tem N·ℓ entradas.
func CifrarGrupo(chaves [][TamChave]byte, pares [][2]Bloco, instancias int) ([][][]byte, error) {
	n := len(chaves)
	l := BitsIndice(n)
	if len(pares) != instancias*l {
		return nil, errors.New("quantidade de OTs não confere com o grupo")
	}
	out := make([][][]byte, instancias)
	ks := make([]Bloco, l)
	for inst := 0; inst < instancias; inst++ {
		meus := pares[inst*l : (inst+1)*l]
		out[inst] = make([][]byte, n)
		for i := 0; i < n; i++ {
			for j, b := range bitsDe(i, l) {
				if b {
					ks[j] = meus[j][1]
				} else {
					ks[j] = meus[j][0]
				}
			}
			p := pad(inst, i, ks)
			c := make([]byte, TamChave)
			for x := range c {
				c[x] = chaves[i][x] ^ p[x]
			}
			out[inst][i] = c
		}
	}
	return out, nil
}

// AbrirGrupo é o lado do receptor: com os blocos recebidos (N·ℓ, um por OT) e
// os índices escolhidos, recupera a chave de cada livro escolhido.
func AbrirGrupo(cifras [][][]byte, recebidos []Bloco, indices []int, n int) ([][TamChave]byte, error) {
	l := BitsIndice(n)
	if len(cifras) != len(indices) || len(recebidos) != len(indices)*l {
		return nil, errors.New("resposta do OT não confere com a compra")
	}
	out := make([][TamChave]byte, len(indices))
	for inst, i := range indices {
		if i < 0 || i >= n || len(cifras[inst]) != n || len(cifras[inst][i]) != TamChave {
			return nil, errors.New("resposta do OT malformada")
		}
		p := pad(inst, i, recebidos[inst*l:(inst+1)*l])
		for x := 0; x < TamChave; x++ {
			out[inst][x] = cifras[inst][i][x] ^ p[x]
		}
	}
	return out, nil
}
