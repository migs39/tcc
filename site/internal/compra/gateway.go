package compra

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// Gateway é o processador de pagamento (RF13). Recebe só o valor e os dados
// de pagamento: nunca os livros (o valor é preço da faixa × N, invariante 4).
type Gateway interface {
	Cobrar(ctx context.Context, valorCentavos int, dados string) (id string, aprovado bool, err error)
}

// GatewayMock aprova tudo, exceto quando os dados de pagamento são "recusar"
// (D-010). Serve para testes e para a demonstração.
type GatewayMock struct{}

func (GatewayMock) Cobrar(_ context.Context, _ int, dados string) (string, bool, error) {
	b := make([]byte, 8)
	rand.Read(b)
	id := "mock_" + hex.EncodeToString(b)
	return id, dados != "recusar", nil
}
