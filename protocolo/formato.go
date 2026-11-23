package protocolo

// Formatos das mensagens entre o programa local e o site (D-012). Binários vão
// em base64 (padrão do encoding/json para []byte).

// MinimoGrupo é o tamanho mínimo de um grupo de mesmo preço (D-010).
const MinimoGrupo = 5

// MaximoPorCompra limita N por compra.
const MaximoPorCompra = 50

// PedidoCompra é o que o programa local envia ao site (passo 2 do fluxo).
// Não contém nada sobre os livros escolhidos além do que o OT esconde (RF07).
type PedidoCompra struct {
	Email         string   `json:"email"`
	VersaoGrupoID int64    `json:"versao_grupo_id"`
	Quantidade    int      `json:"quantidade"`
	RequisicaoOT  []byte   `json:"requisicao_ot"`
	Cegados       [][]byte `json:"cegados"`
	Pagamento     string   `json:"pagamento"` // dados para o gateway (mock: "aprovar"/"recusar")
}

// RespostaOT é a resposta do OT de uma compra: o que vai por e-mail (RF14) e
// no reenvio (RF15). Não contém nenhuma chave: sem a semente local ela não
// revela nada (invariante 2). As assinaturas cegas vão junto para a
// recuperação após falha (D-012); são exatamente o que o site já viu.
type RespostaOT struct {
	Formato          int        `json:"formato"`
	CompraID         string     `json:"compra_id"`
	VersaoGrupoID    int64      `json:"versao_grupo_id"`
	Quantidade       int        `json:"quantidade"`
	RespostaSender   []byte     `json:"resposta_sender"`
	Cifras           [][][]byte `json:"cifras"`
	AssinaturasCegas [][]byte   `json:"assinaturas_cegas"`
}

// FormatoResposta identifica RespostaOT.
const FormatoResposta = 1

// ResultadoCompra é a resposta HTTP do site à compra.
type ResultadoCompra struct {
	Status   string      `json:"status"` // "aprovada" ou "recusada"
	Mensagem string      `json:"mensagem,omitempty"`
	Resposta *RespostaOT `json:"resposta,omitempty"`
}

// ReporteCDN é o lote que a CDN envia ao site (RF18, D-011): só tokens e
// quantidade, sem livro e sem horários, em ordem aleatória.
type ReporteCDN struct {
	Tokens     []string `json:"tokens"`
	Quantidade int      `json:"quantidade"`
}
