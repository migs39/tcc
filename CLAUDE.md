# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Estado do repositório

Ainda não há código: o repositório contém apenas `README.md` e `Especificacao_Detalhada.pdf` (especificação do TCC, fonte de verdade para requisitos). Não existem comandos de build, lint ou testes definidos — atualize esta seção quando a stack for escolhida.

## Projeto

TCC (PCS/Poli-USP, 2026): livraria digital em modelo de catálogo que usa **Oblivious Transfer (OT)** para vender chaves simétricas de livros cifrados, de modo que o servidor da loja não saiba quais livros foram comprados.

## Arquitetura prevista (três componentes)

1. **Sistema Web (site)** — catálogo, administração, preços discretos, pagamento via gateway, execução do OT e assinatura cega de tokens. Compra sem conta; só pede e-mail.
2. **CDN** — armazena os livros cifrados; entrega um livro mediante um token com assinatura cega válido. Depois reporta os tokens usados ao site para ser remunerada.
3. **Executável local offline** — guarda o estado secreto do cliente e descriptografa os livros sem acesso à internet.

Fluxo de compra: o cliente escolhe um *grupo de livros de mesmo preço* e obtém, via OT, as chaves de N itens do grupo. Recebe também N tokens com assinatura cega, baixa os livros cifrados na CDN com os tokens e os descriptografa localmente.

## Modelo de ameaça (decisões já tomadas)

- **Site e CDN não são coniventes** (premissa garantida por auditoria). O site sabe *quem* comprou, mas não *o quê*. A CDN sabe *o quê* foi baixado, mas não *quem* comprou. Não trate essa separação como falha a ser corrigida.
- Consequências a respeitar no design:
  - o reporte CDN → site deve conter apenas tokens e quantidade, nunca o livro associado a cada token nem horários que permitam correlação;
  - os tokens devem ser impossíveis de ligar à sessão de assinatura (é para isso que serve a assinatura cega).
- O servidor **não conhece a chave final** do cliente. O e-mail (RF14) e o reenvio (RF15) carregam a *resposta do OT*, não a chave. O cliente (executável local) precisa guardar o segredo da requisição para extrair a chave.

## Requisitos-chave

- RNF01: segurança criptográfica de pelo menos 128 bits.
- Metas de desempenho: busca em até 5 s, finalização da compra em até 1 min, envio do e-mail em até 1 min, 600 transações/min.
- Primitivas estudadas na especificação: OT 1-de-2, 1-de-n, k-de-n, GOT (Tassa 2011), OT Extension e criptossistema de Rabin (exige p ≡ q ≡ 3 mod 4 para a fórmula de raiz usada).
