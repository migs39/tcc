package protocolo

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// OTHelper chama o binário ot-helper (libOTe, MR19; D-002). Caminho é o
// executável; vazio significa "ot-helper" no PATH.
type OTHelper struct{ Caminho string }

const limiteOT = 30 * time.Second

func (o OTHelper) rodar(ctx context.Context, entrada string, args ...string) ([]string, error) {
	ctx, cancelar := context.WithTimeout(ctx, limiteOT)
	defer cancelar()
	bin := o.Caminho
	if bin == "" {
		bin = "ot-helper"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = strings.NewReader(entrada + "\n")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ot-helper %s: %v: %s", args[0], err, strings.TrimSpace(errb.String()))
	}
	return strings.Fields(strings.ReplaceAll(out.String(), "\r", "")), nil
}

// Sender roda o lado do sender para m OTs: recebe a requisição do receptor e
// devolve a resposta e os m pares (K^0, K^1). A aleatoriedade é do sistema.
func (o OTHelper) Sender(ctx context.Context, requisicao []byte, m int) ([]byte, [][2]Bloco, error) {
	campos, err := o.rodar(ctx, hex.EncodeToString(requisicao), "sender", strconv.Itoa(m))
	if err != nil {
		return nil, nil, err
	}
	if len(campos) != 1+2*m {
		return nil, nil, errors.New("ot-helper: saída do sender inesperada")
	}
	resp, err := hex.DecodeString(campos[0])
	if err != nil {
		return nil, nil, err
	}
	pares := make([][2]Bloco, m)
	for i := 0; i < m; i++ {
		for b := 0; b < 2; b++ {
			if err := blocoHex(campos[1+2*i+b], &pares[i][b]); err != nil {
				return nil, nil, err
			}
		}
	}
	return resp, pares, nil
}

// Requisicao gera a requisição do receptor a partir da semente (D-001).
func (o OTHelper) Requisicao(ctx context.Context, semente [16]byte, escolhas string) ([]byte, error) {
	campos, err := o.rodar(ctx, "", "requisicao", hex.EncodeToString(semente[:]), escolhas)
	if err != nil {
		return nil, err
	}
	if len(campos) != 1 {
		return nil, errors.New("ot-helper: saída da requisição inesperada")
	}
	return hex.DecodeString(campos[0])
}

// Extrair refaz o receptor com a mesma semente e escolhas e devolve o bloco
// escolhido de cada OT.
func (o OTHelper) Extrair(ctx context.Context, semente [16]byte, escolhas string, resposta []byte) ([]Bloco, error) {
	campos, err := o.rodar(ctx, hex.EncodeToString(resposta), "extrair", hex.EncodeToString(semente[:]), escolhas)
	if err != nil {
		return nil, err
	}
	if len(campos) != len(escolhas) {
		return nil, errors.New("ot-helper: saída da extração inesperada")
	}
	out := make([]Bloco, len(campos))
	for i, c := range campos {
		if err := blocoHex(c, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func blocoHex(s string, b *Bloco) error {
	v, err := hex.DecodeString(s)
	if err != nil || len(v) != len(b) {
		return errors.New("ot-helper: bloco inválido")
	}
	copy(b[:], v)
	return nil
}
