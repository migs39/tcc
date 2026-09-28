# PRD — o que precisa ser atualizado

Data: 2026-09-27
Base: `PRD_Livraria_OT.pdf` (27/09/2026, @M4GAL). Referência: especificação **v3** (`Especificacao_Detalhada_v3.pdf`), que vence em caso de divergência.

O PRD original é um documento de outra pessoa do grupo e não foi refeito. Esta lista serve para quem for editá-lo. Ela segue as seções do PRD, na ordem em que aparecem.

Origem de cada mudança:
- decisões D-001 a D-004: `CLAUDE.md`;
- divergências D1–D3 e lacunas L1–L5: `docs/divergencias_docs.md`.

---

## Visão geral e problema

- **Proposta:** trocar "descriptografados offline, em um executável local" por "descriptografados localmente, em um programa instalado no computador do cliente". (D-004)

## Objetivos e não objetivos

- **Objetivo 4:** "O cliente descriptografa os livros comprados sem acesso à internet" → "O cliente descriptografa os livros comprados localmente; a chave nunca sai do seu computador". (D-004, D3)
- **Objetivo 1:** continua igual. Vale acrescentar que isso inclui a navegação: o site entrega o catálogo inteiro e o filtro roda no cliente. (L1)

## Usuários

- **Administrador, "O que precisa fazer":** "Gerir a própria conta" → "Gerir contas de administradores (a própria e as de outros)". (D1, RF01 da especificação)

## Arquitetura e fluxo de compra

**Diagrama:**
- A caixa "Executável local — Offline; guarda o segredo do OT" vira "Programa local — guarda a semente do OT; extrai a chave e decifra".
- As setas 2 e 4 passam a ligar o **programa local** ao site, não o navegador.
- A seta 5 (token por livro cifrado) passa a sair do programa local para a CDN.

**Passos:**
1. "O executável local gera a requisição de OT e guarda o segredo dela" → "O programa local sorteia uma semente nova, gera a requisição de OT e os N valores cegados, e guarda semente + escolhas".
2. "O cliente envia ao site…" → "O programa local envia ao site o e-mail, a requisição de OT e os N valores cegados".
3. Sem mudança.
4. Sem mudança no conteúdo; o destino é o programa local.
5. "O cliente troca cada token…" → "O programa local remove o cegamento e troca cada token por um livro cifrado na CDN".
6. Sem mudança.
7. "Offline, o executável extrai as chaves…" → "O programa local extrai as chaves com a semente e decifra os livros".

**Frase abaixo do fluxo:** "…dentro do executável, que é o único a guardar o segredo da requisição" → "…dentro do programa local, que é o único a guardar a semente da requisição".

**Parágrafo novo (D-001):** se a compra for interrompida depois do pagamento, o programa local reproduz o receptor a partir da semente e extrai as chaves usando a resposta do OT recebida por e-mail.

## Requisitos funcionais

| ID | Mudança | Origem |
|---|---|---|
| RF04 | Requisito: acrescentar "o catálogo é entregue inteiro e filtrado no cliente". Critério de aceite: acrescentar "o site não recebe os termos de busca". | L1 |
| RF07 | Requisito: acrescentar "sobre uma versão imutável do grupo". Critério de aceite: acrescentar "mudanças no grupo durante a compra não alteram os livros escolhidos". | L2 |
| RF09 | "Arquitetura com site, CDN e executável local" → "…e programa local". | D-004 |
| RF10 | Requisito: "Descriptografar offline" → "Descriptografar localmente". Critério de aceite: "Funciona sem nenhuma conexão de rede" → "A chave nunca é transmitida; o programa só se conecta ao site e à CDN". | D-004, D3 |
| RF11 | Requisito: "Gerar a requisição de OT e guardar o segredo dela" → "…a partir de uma semente (≥ 128 bits, nova a cada compra) e guardar semente + escolhas". Critério de aceite: acrescentar "com a semente e a resposta do e-mail, as chaves podem ser extraídas mesmo após interrupção". | D-001 |
| RF12 | Componente "Executável" → "Programa local". | D-004 |
| **Novo** (lado do site do RF18 / UC17) | Requisito: "Validar os tokens reportados pela CDN e remunerá-la". Critério de aceite: "Token com assinatura inválida ou já reportado é rejeitado". Prioridade: Essencial. | D2 |

Nas linhas RF10–RF12, trocar a coluna "Componente" de "Executável" para "Programa local".

## Requisitos não funcionais

- Sem mudança.

## Modelo de ameaça e garantias

**Tabela "Parte / Sabe / Não sabe / Garantido por":**
- Linha "E-mail do cliente", coluna "Garantido por": "Segredo guardado no executável (RF11)" → "Semente guardada no programa local (RF11)".
- **Linha nova, opcional:** CDN | sabe o IP de quem baixa | não sabe quem comprou | premissa de não conivência.

**Regras de design (acrescentar):**
- O código que gera a requisição de OT roda no programa local, **nunca** é servido pelo site. Por isso a privacidade da escolha é garantia do protocolo, não premissa.
- O programa local só se conecta ao site e à CDN: sem telemetria, sem terceiros.
- O cegamento dos tokens é removido no programa local. (L5)
- O site não registra navegação nem usa scripts ou recursos de terceiros. (L1; medida de engenharia, não garantia)

## Premissas, riscos e questões em aberto

**Premissas:**
- **Remover:** "O cliente consegue levar arquivos entre o navegador e o executável offline". Deixou de valer com D-004.
- **Acrescentar:** "O cliente instala o programa local e mantém a semente de cada compra".

**Riscos:**

| Risco | Mudança |
|---|---|
| OT de N itens caro demais | Mitigação: "libOTe com OT base Masny–Rindal; benchmark nos testes da implementação". |
| Cliente perde o segredo local | "segredo local" → "semente". Mitigação: backup da semente, cuja forma de proteção está em aberto. |
| **Novo:** libOTe não compila no Windows ou o receptor não é determinístico pela semente | Efeito: D-001/D-002 precisam ser revistas. Mitigação: testes automatizados no passo 6. Risco aceito pelo grupo, sem spike. |
| **Novo:** canal de tempo entre compra e download | Efeito: o programa baixa da CDN logo após a compra. Mitigação: reporte da CDN em lotes, sem horários (decisão em aberto). |

**Questões em aberto:**
- **Continuam:** variante de OT; tamanho mínimo de grupo; prazo de reenvio (RF15); token de uso único; frequência e remuneração do reporte; algoritmo simétrico.
- **Acrescentar:**
  - política de chaves da assinatura cega (conjunto de anonimato, L4);
  - gateway de pagamento (mock ou sandbox) e como o pagamento se encaixa com o programa local;
  - proteção da semente em disco;
  - formato dos artefatos.

## Escopo do MVP e marcos

- **Marco 3 (Projeto da solução):** marcar como "em andamento". Já decididos:
  - OT síncrono com semente (D-001);
  - libOTe/MR19 (D-002);
  - stack do site: Go, PostgreSQL, HTML no servidor (D-003);
  - programa local com rede restrita (D-004).
- **Marco 4e:** "executável local: segredo, extração da chave e descriptografia" → "programa local: semente, extração da chave e descriptografia".
- **Fonte citada no início do PRD:** "Especificação Detalhada, versão 2" → "versão 3".
