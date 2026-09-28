# Divergências e lacunas entre PRD e Especificação

Fontes: `Especificacao_Detalhada_v2.pdf` (especificação) e `PRD_Livraria_OT.pdf` (PRD de 27/09/2026).
Atualização 2026-09-27: a especificação **v3** (`Especificacao_Detalhada_v3.pdf`) já incorpora D3, L1, L2, L3 e L5. O PRD ainda não foi atualizado.
Regra: quando divergem, **a especificação vence**. Este arquivo registra o que foi encontrado para corrigir os documentos.

Legenda de status: `aberto` (falta corrigir o documento ou decidir) · `resolvido` (documento corrigido ou decisão registrada no CLAUDE.md).

---

## 1. Divergências (PRD × especificação)

### D1 — Escopo da gestão de administradores (RF01, UC01) — `aberto`
- **PRD (tabela de usuários):** o administrador "gere a **própria** conta".
- **Especificação (RF01, UC01):** "cadastro, autenticação e gerenciamento de **perfis de administradores**", ou seja, um admin também gerencia outros admins.
- **Adotado:** especificação. O site permite a um admin cadastrar e gerenciar outros admins.
- **Ação:** ajustar a tabela de usuários do PRD.

### D2 — Validação do reporte e remuneração da CDN (UC17) — `aberto`
- **Especificação:** UC17 ("Validar tokens reportados e remunerar a CDN") e a Figura 2 (alt tokens válidos/inválidos) atribuem ao site a validação e o pagamento.
- **PRD:** não há RF nem critério de aceite para isso. RF18 cobre só o lado da CDN (enviar o reporte).
- **Adotado:** especificação. O site implementa o recebimento e a validação do reporte (plano do site, passo 8).
- **Ação:** incluir no PRD o lado do site do RF18/UC17, com critério de aceite (ex.: token com assinatura inválida ou já reportado é rejeitado).

### D3 — Programa local deixa de ser offline (RF09, RF10) — `especificação v3 atualizada; falta o PRD`
- **Especificação:** RF09 descreve "um executável local offline"; RF10 exige que ele "não se conecta à internet". O PRD repete isso (objetivo 4, RF10, fluxo passo 7).
- **Projeto (D-004 no CLAUDE.md, 2026-09-27):** o programa local pode usar rede, **só** com o site e a CDN. A descriptografia continua local e a chave nunca trafega.
- **Motivo:** usabilidade (RNF07). A privacidade da escolha (RF07) continua garantida formalmente, porque o código do receptor do OT não vem do site.
- **Ação:** reescrever RF10 (ex.: "descriptografia local; o programa se conecta apenas ao site e à CDN") e a descrição do RF09 na especificação e no PRD; revisar o objetivo 4 do PRD ("sem acesso à internet").

---

## 2. Lacunas (nenhum dos dois documentos cobre)

### L1 — Busca e navegação vazam interesse do cliente (RF04 × objetivo 1 do PRD) — `resolvido (D-003; especificação v3, RF04 e 4.3)`
- O objetivo 1 do PRD diz que o site não pode identificar o que o comprador "adquiriu **ou acessou**".
- A busca no servidor (RF04) e as páginas de detalhe por livro mostram ao site o que cada visitante consultou. Na compra, o site recebe o e-mail. Por sessão, IP ou tempo, ele pode ligar interesse a pessoa.
- O OT não cobre esse canal. Qualquer proteção aqui é **escolha de engenharia**, não garantia formal.
- **Opções:** (a) entregar o catálogo inteiro num pacote único e filtrar no navegador; (b) busca no servidor sem logs nem cookies (mais fraca, depende de disciplina operacional).
- **Ação:** decidir e registrar o canal na especificação (modelo de ameaça, seção 4.3).

### L2 — Estabilidade dos índices do grupo durante a compra (RF02 × RF07) — `resolvido (especificação v3, RF07 e 4.4)`
- O OT trabalha com índices `i ∈ {0..n-1}` do grupo. Se o admin altera o grupo (RF02: cadastrar ou remover livro, mudar preço) entre o passo 1 e o passo 4 do fluxo, o índice escolhido passa a apontar para outro livro.
- **Proposta:** o grupo tem **versão/snapshot** imutável, e a requisição OT referencia essa versão.
- **Ação:** incluir na especificação (Projeto Proposto, 4.4) e na definição do formato da requisição OT.

### L3 — Padrão de mensagens do OT imposto pelo executável offline (RF10, RF11 × fluxo) — `resolvido (D-001, D-002; especificação v3, 4.4)`
- Pelo fluxo (Fig. 1), o **executável gera a requisição** (offline), o site responde uma vez e o executável extrai offline, possivelmente muito depois (a resposta também chega por e-mail, RF14).
- **Requisito derivado:** (a) o receptor nunca fala direto com o site; cada mensagem é um arquivo que o navegador transporta; (b) o **estado do receptor** (escolhas + segredos aleatórios) é **serializável** e fica guardado no executável entre gerar a requisição e extrair as chaves (RF11); (c) o site responde em **uma única mensagem**.
- Uma primeira mensagem do sender que **não dependa do receptor** (ex.: o ponto público do sender em CO15/MR19) é aceitável: o navegador a baixa junto com o snapshot do grupo, antes do passo 1. Protocolos em que o receptor fala primeiro (ex.: PVW) encaixam direto.
- Protocolos com várias idas e vindas por compra (OT Extension IKNP/KOS/KKRT/Silent) não encaixam.
- **Obs. de implementação:** as bibliotecas de OT em C++ (libOTe, emp-ot) executam o protocolo numa única chamada/corrotina com o estado em memória, sem API para serializá-lo. Usá-las exige um adaptador (ver levantamento de OT).
- **Ação:** registrar como requisito derivado na especificação e usar como critério na decisão da variante de OT.

### L4 — Conjunto de anonimato dos tokens (RF16) — `aberto`
- Os tokens só são não ligáveis dentro do conjunto assinado com a **mesma chave**. Uma chave por grupo ou por período, ou rotação frequente, reduz esse conjunto e enfraquece o RF16 na prática.
- **Ação:** definir a política de chaves de assinatura cega (quantas, por quanto tempo) junto com o formato dos tokens.

### L5 — Onde acontece o cegamento/descegamento dos tokens (RF16, passo 5) — `resolvido (D-004: no programa local; especificação v3, 4.3 e Fig. 1)`
- A Fig. 1 mostra "Remover cegamento: N tokens" no **Cliente**, sem dizer se é no navegador ou no executável.
- **Impacto:** no navegador, o site precisa servir código criptográfico (JS), e o site controla esse código. No executável, o navegador só transporta arquivos (premissa do PRD).
- **Ação:** decidir e registrar na especificação.
