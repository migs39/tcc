# Especificação Detalhada — alterações da v2 para a v3

Data: 2026-09-27
Base: `Especificacao_Detalhada_v2.pdf`. Origem das mudanças: decisões D-001 a D-004 (`CLAUDE.md`) e as divergências e lacunas de `docs/divergencias_docs.md`.

Cada item traz **onde** mudar, **por quê** e o **texto novo em LaTeX**, pronto para colar no fonte (o PDF da v2 foi gerado com abnTeX2).
- `[pendente]` marca pontos que dependem de decisão em aberto: o texto mostra só o que já está decidido.
- Itens sem mudança ficam como estão.

---

## Resumo das mudanças

| # | Seção | Mudança | Origem |
|---|---|---|---|
| 1 | 1.2 Objetivos | "executável local offline" → "programa local" | D-004 |
| 2 | 3.4 Fase 3 | Nome do terceiro componente | D-004 |
| 3 | 4.1 RF04 | Catálogo entregue inteiro, filtro no cliente | L1, D-003 |
| 4 | 4.1 RF07 | Grupo versionado | L2 |
| 5 | 4.1 RF09 | Terceiro componente: programa local, conecta só ao site e à CDN | D-004 |
| 6 | 4.1 RF10 | Deixa de exigir "não se conecta à internet"; descriptografia local | D-004 (D3) |
| 7 | 4.1 RF11 | Segredo = semente + escolhas; reextração a partir da resposta por e-mail | D-001 |
| 8 | 4.3 Modelo de ameaça | Quatro premissas/propriedades novas | L1, L5, D-004 |
| 9 | 4.4 Projeto proposto | Componente 3 renomeado; execução do OT; grupo versionado | D-001, D-004, L2, L3 |
| 10 | 4.5 Casos de uso e figuras | "Executável Local" → "Programa Local"; mensagens diretas ao site e à CDN | D-004, L5 |
| 11 | Novo cap. 5 (esboço) | Decisões de projeto e tecnologias | D-001 a D-004 |

---

## 1. Seção 1.2 — Objetivos específicos (2º item)

**Por quê:** D-004.

```latex
\item Implementar o algoritmo criptográfico escolhido (como 1-out-of-N) em uma
solução com 3 componentes: site online, rede de distribuição de conteúdo (CDN)
e programa local instalado no computador do cliente.
```

## 2. Seção 3.4 — Fase 3

**Por quê:** D-004. Trocar apenas o trecho entre parênteses:

```latex
organização em três componentes (sistema web, CDN e programa local),
```

## 3. RF04 — Busca e navegação no catálogo

**Por quê:** a busca no servidor mostraria ao site o que cada cliente procura (lacuna L1). Isso contraria o objetivo de o site não saber o que o comprador "adquiriu ou acessou".

```latex
\item \textbf{RF04 --- Busca e navegação no catálogo:} o sistema deve permitir
que clientes pesquisem livros por critérios como título, autor, categoria e
preço. O catálogo é entregue por inteiro, na mesma forma para todos os
clientes, e a filtragem é feita no próprio cliente, de modo que o servidor não
observe os critérios de busca nem os livros consultados.
```

## 4. RF07 — Compra privada de N itens em grupo

**Por quê:** os índices do OT dependem da ordem do grupo. Se o grupo mudar durante a compra, o índice passa a apontar para outro livro (lacuna L2).

```latex
\item \textbf{RF07 --- Compra privada de N itens em grupo:} o cliente deve
conseguir comprar N livros dentre um grupo maior de livros com o mesmo preço
sem que a informação dos itens específicos comprados seja enviada ao servidor
em formato que ele consiga decifrar. Cada grupo possui uma versão imutável
(composição e ordem dos livros), e a compra referencia essa versão.
```

## 5. RF09 — Arquitetura com três componentes

**Por quê:** D-004. O item (iii) muda; (i) e (ii) ficam iguais.

```latex
\item \textbf{RF09 --- Arquitetura com três componentes:} a solução deve ser
composta por (i) um sistema web para catálogo, administração, pagamento,
execução do OT e assinatura cega de tokens; (ii) uma CDN, que armazena os
livros cifrados e os entrega mediante token válido; e (iii) um programa local,
instalado no computador do cliente e não fornecido pelo sistema web em tempo de
execução, que guarda o estado secreto do cliente, comunica-se apenas com o
sistema web e com a CDN e descriptografa os livros.
```

## 6. RF10 — Descriptografia local

**Por quê:** D-004 (divergência D3). O requisito deixa de exigir que o programa fique sem internet. A propriedade que importa continua: a chave só existe na máquina do cliente.

```latex
\item \textbf{RF10 --- Descriptografia local:} o cliente deve conseguir
descriptografar o item resgatado por meio do programa local, a partir do livro
cifrado e da resposta do OT. A chave do livro é obtida e usada apenas no
computador do cliente e nunca é transmitida pela rede. O programa local
comunica-se exclusivamente com o sistema web e com a CDN, sem telemetria nem
serviços de terceiros.
```

## 7. RF11 — Gestão do segredo no programa local

**Por quê:** D-001. O segredo passa a ser uma semente da qual deriva toda a aleatoriedade do receptor. Isso permite reextrair as chaves se a compra falhar depois do pagamento.

```latex
\item \textbf{RF11 --- Gestão do segredo no programa local:} o programa local
deve gerar a requisição de OT a partir de uma semente secreta, obtida de um
gerador criptograficamente seguro do sistema, com pelo menos 128 bits, nova a
cada compra e nunca reutilizada. O programa guarda a semente e as escolhas,
necessárias para extrair, a partir da resposta do servidor, as chaves
criptográficas dos itens adquiridos, inclusive a partir da resposta recebida
por e-mail (RF14) caso a compra seja interrompida após o pagamento. A perda da
semente implica a perda de acesso aos itens daquela compra.
```

## 8. Seção 4.3 — Modelo de ameaça (novos itens)

**Por quê:** L1, L5 e D-004. Acrescentar ao final da lista. Os quatro itens existentes ficam como estão; no último deles, trocar "executável local" por "programa local".

```latex
\item \textbf{O código do receptor do OT não vem do servidor.} A requisição de
OT é gerada pelo programa local, instalado pelo cliente, e não por código
entregue pelo sistema web durante a compra. Assim, a privacidade da escolha
(RF07) decorre do protocolo de OT e não depende de o servidor fornecer código
honesto.

\item \textbf{Navegação e busca não revelam interesse.} O catálogo é entregue
por inteiro e filtrado no cliente (RF04). Trata-se de uma medida de engenharia,
não de uma garantia do protocolo; complementam-na a ausência de registros de
navegação e de scripts ou recursos de terceiros no sistema web.

\item \textbf{O programa local fala apenas com o sistema web e com a CDN.} A
chave final e a semente nunca deixam o computador do cliente (RF10, RF11). O
endereço de rede do cliente é visível à CDN; a ligação entre esse dado e a
identidade do comprador exigiria conivência entre site e CDN, excluída pela
premissa acima.

\item \textbf{O cegamento dos tokens é removido no programa local.} Os valores
cegados são gerados e descegados no programa local, e não no navegador, pelo
mesmo motivo do primeiro item.
```

**`[pendente]`** (não alterar ainda): a política de chaves da assinatura cega e o tamanho do conjunto de anonimato (lacuna L4); os lotes do reporte da CDN e o momento dos downloads logo após a compra (canal de tempo).

## 9. Seção 4.4 — Projeto proposto

**Por quê:** D-004, D-001, L2 e L3.

Substituir o item 3 da lista:

```latex
\item \textbf{Programa local:} instalado no computador do cliente, gera a
requisição de OT e guarda seu segredo, comunica-se com o sistema web e com a
CDN, remove o cegamento dos tokens e descriptografa os livros localmente.
```

Acrescentar após o parágrafo final:

```latex
A execução do OT é síncrona: durante a compra, o programa local envia a
requisição de OT e recebe a resposta em uma única troca de mensagens com o
sistema web, dentro do limite de RNF03. Toda a aleatoriedade do lado receptor é
derivada de uma semente guardada pelo programa local (RF11); por isso, se a
compra for interrompida após o pagamento, o programa reproduz o estado do
receptor a partir da semente e extrai as chaves usando a resposta do OT
recebida por e-mail (RF14). Esse desenho exige um protocolo de OT em que o
sistema web responda em uma única mensagem e em que o estado do receptor seja
reproduzível a partir da semente.

Os livros de um grupo de mesmo preço são organizados em versões imutáveis: a
inclusão, remoção ou alteração de preço de um livro gera uma nova versão do
grupo, e cada compra referencia a versão sobre a qual os índices do OT foram
escolhidos.
```

## 10. Seção 4.5 — Casos de uso e figuras

**Por quê:** D-004. Com isso, a lacuna L5 (onde o cegamento é removido) fica resolvida.

**Tabela 1:**
- UC18: agente "Cliente (Programa Local)".
- UC19: agente "Sistema (Programa Local)".
- UC14: "Baixar livro cifrado na CDN apresentando um token (via programa local)".

**Figura 1:**
- Renomear a linha de vida "Executável Local" para "Programa Local".
- A mensagem "Requisição OT" deixa de voltar ao Cliente. Passa a ir do Programa Local ao Sistema Web, junto com e-mail e N valores cegados.
- "Resposta do OT + N assinaturas cegas" volta ao Programa Local.
- "Remover cegamento: N tokens" passa a ser autoinvocação do Programa Local.
- O Cliente continua escolhendo o grupo e os N livros, agora no Programa Local.

**Figura 2:**
- No bloco "Entrega", as mensagens com a CDN partem do Programa Local.
- Renomear o bloco "Descriptografia (offline)" para "Descriptografia (local)".
- A mensagem "Inserir resposta do OT + livro cifrado" some no fluxo normal: o programa já os tem. Ela continua valendo para o caso de retomada a partir do e-mail.

**Texto abaixo da tabela:** trocar "descriptografia offline" por "descriptografia local".

## 11. Novo capítulo 5 — Projeto da solução (esboço)

A v2 remete o detalhamento da Fase 3 ao capítulo 5, que ainda não existe. O esboço abaixo registra as decisões já tomadas.

```latex
\chapter{Projeto da Solução}

\section{Decisões de projeto}

\begin{itemize}
\item \textbf{Execução do OT:} síncrona na compra, com o estado do receptor
reproduzível a partir de uma semente guardada pelo programa local (RF11,
RF14).

\item \textbf{Biblioteca de OT:} libOTe, com o protocolo de
Masny e Rindal como OT base 1-de-2. A compilação no ambiente do cliente e a
reprodutibilidade do receptor a partir da semente são verificadas por testes
automatizados durante a implementação.

\item \textbf{Sistema web:} linguagem Go, banco PostgreSQL, páginas
renderizadas no servidor, sem recursos de terceiros; catálogo entregue por
inteiro e filtrado no cliente; biblioteca de OT acessada por um binário
auxiliar.

\item \textbf{Assinatura cega:} RSA cego padronizado (RFC 9474).

\item \textbf{Programa local:} conecta-se apenas ao sistema web e à CDN.
\end{itemize}

\section{Alternativas descartadas}
% emp-ot (sem suporte a Windows); simot (sender fala primeiro, apenas 1-de-2);
% mpz e swanky (não recomendados para produção pelos autores);
% OT via OPRF (fora do foco do trabalho);
% receptor no navegador (a privacidade da escolha passaria a depender de o
% servidor fornecer código honesto).
```

**`[pendente]`** para o capítulo 5:
- variante de OT (1-de-n repetido, k-de-n ou GOT);
- algoritmo simétrico;
- formatos dos artefatos;
- tamanho mínimo de grupo;
- política de chaves dos tokens;
- lotes do reporte;
- prazo de reenvio;
- gateway.

---

## Fora da especificação: correções no PRD

As divergências D1 (gestão de admins) e D2 (UC17 sem RF) são erros do PRD, não da especificação. Estão em `docs/divergencias_docs.md`. O PRD também precisa refletir a D3: objetivo 4 ("sem acesso à internet"), RF10 e o passo 7 do fluxo.
