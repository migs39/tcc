# Roteiro — Atividade 4: evolução do projeto (2), 5 min

Slides: https://claude.ai/artifact/8nJBV8XmfnN9Mgvx5CFigW
Duração total: cerca de 4 min 50 s, com 9 slides. As falas abaixo também estão nas notas de cada slide.

**Combinar antes:** quem apresenta cada bloco. Sugestão: uma pessoa nos slides 1–3 (contexto e fluxo), outra nos 4–6 (decisões, sistema no ar e testes) e outra nos 7–9 (desempenho, privacidade e próximos passos).

**Ponto de partida:** na Atividade 3, só o site existia (catálogo, admin e pacote assinado). Esta mostra o sistema completo, a avaliação e o que falta até a banca.

**Entrega no e-Disciplinas (véspera):** a monografia quase final (`docs/monografia/main.pdf`) e a apresentação. Para baixar o deck, use Share › Export, como PowerPoint ou PDF.

---

## 1. Capa — 15 s · [quem: ______]

> "Na última apresentação, o site estava começando. Hoje o sistema está completo, de ponta a ponta, testado e no ar. Vamos mostrar como a compra funciona, o que medimos e o que cada proteção de privacidade garante."

## 2. Onde estamos — 30 s · [quem: ______]

> "Das seis fases do método, cinco estão concluídas. A sexta, a consolidação, é a monografia, que está quase final. Agora os três componentes rodam, os 18 requisitos funcionais estão implementados, e a bateria de testes passa inteira: 24 casos de ponta a ponta e 48 testes automatizados."

## 3. A compra privada — 45 s · [quem: ______]

> "Este é o fluxo que agora roda inteiro. O programa local gera a requisição do OT e grava a semente antes de enviar. O site cobra pelo grupo e pela quantidade, roda o lado sender do OT e assina às cegas os tokens de download. Com os tokens, o programa baixa os livros cifrados na CDN, que depois reporta ao site só quantos tokens foram usados. **A chave só aparece no passo 7, dentro do programa, e nunca passa pela rede.** Se a compra cair depois do pagamento, a semente e a resposta que chega por e-mail bastam para concluir."

## 4. Decisões — 40 s · [quem: ______]

> "Na última apresentação, a variante de OT era a principal decisão em aberto. Ficamos com o 1-de-n de Naor e Pinkas sobre o OT base do libOTe: cada livro comprado custa log de n OTs de 1-de-2, o que se mostrou barato. Os tokens usam RSA cego padronizado, com uma chave por faixa publicada no catálogo assinado, para que o site não marque os tokens de um cliente. Para pôr o sistema na internet, usamos um túnel em que a conexão cifrada termina na nossa máquina: um túnel que decifrasse o tráfego veria a compra e o download do mesmo cliente."

## 5. Demonstração — 35 s · [quem: ______]

> "O sistema está no ar. A loja tem uma página pública com o hash e a chave do catálogo. O programa para Windows é baixado do Release do GitHub, e não da loja, de propósito: se a loja servisse o programa, poderia entregar a alguém uma versão adulterada. E a resposta do OT chega por e-mail de verdade, sem chave nenhuma."

**Se der tempo:** abrir a janela, escolher dois livros do mesmo grupo, comprar e mostrar a biblioteca (cerca de 20 s). Deixar o `iniciar_demo.bat` rodando antes.

## 6. Testes — 35 s · [quem: ______]

> "A bateria sobe um ambiente isolado, cadastra um catálogo conhecido e usa o programa como um cliente usaria. São 24 casos, cada um ligado a um requisito, e os livros decifrados são comparados byte a byte com os originais. Os testes de privacidade não provam as garantias, que vêm dos protocolos: eles verificam que a implementação não as anula por um canal lateral, como o tamanho de uma resposta ou um log."

## 7. Desempenho — 35 s · [quem: ______]

> "A busca leva 85 milissegundos, e a compra completa, menos de um terço de segundo. No site, o teste concorrente chegou a 1.213 compras por minuto com quatro núcleos, o dobro da meta. **Sendo transparentes:** com um núcleo só, são 396 por minuto, abaixo da meta. O gargalo são as assinaturas cegas."

## 8. Privacidade — 40 s · [quem: ______]

> "Como antes, separamos prova, premissa e engenharia. As três primeiras linhas vêm de provas: o OT, sob o modelo de oráculo aleatório para o lado do cliente, e a assinatura cega. A não conivência entre site e CDN é premissa, e o reporte da CDN não leva horário justamente para não facilitar o cruzamento. A busca local e o comportamento uniforme diante de uma chave errada são escolhas de engenharia. O limite que permanece é o conjunto de anonimato: com poucos compradores numa faixa, ele é pequeno."

## 9. Até a banca — 25 s · [quem: ______]

> "O sistema está fechado; falta documentação e divulgação: pôster e press release no dia 1º, monografia final no dia 8, vídeo no dia 9 e as bancas nos dias 15 e 16. Para continuar o trabalho, os pontos principais são um gateway real, compromissos das chaves no catálogo e assinaturas cegas mais rápidas. Obrigado."

---

## Perguntas prováveis

**"Por que Naor–Pinkas e não k-de-n ou OT Extension?"**
É a construção que a especificação cita, e o libOTe entrega exatamente os OTs 1-de-2 de que ela precisa. O custo é N·⌈log₂ n⌉ OTs base por compra; com os grupos do protótipo, isso é pequeno e dispensa OT Extension. k-de-n exigiria mais código criptográfico próprio.

**"O cliente pode escolher o mesmo livro duas vezes?"**
Pode, e só se prejudica: paga por dois e recebe um. O site não tem como impedir sem aprender a escolha.

**"E se o site responder com chaves erradas para descobrir a escolha?"**
É o ataque de falha seletiva. O AES-GCM detecta a chave errada, e o programa se comporta igual na rede em qualquer caso: troca todos os tokens e não avisa o site. É escolha de engenharia, não garantia; compromissos das chaves no catálogo estão nas perspectivas de continuidade.

**"Site e CDN juntos descobrem tudo?"**
Juntando horário e endereço, podem cruzar compra e download. Por isso a não conivência é premissa, e o reporte da CDN não leva horário nem livro. O programa permite adiar o download.

**"E se o cliente perder o computador?"**
Perde o acesso: a semente é o segredo da compra, e o e-mail sozinho não abre nada. É intencional (invariante 6); a recomendação é a cifragem de disco do sistema.

**"Por que o programa não é servido pelo site?"**
Se fosse, um site malicioso poderia entregar a um cliente específico um programa adulterado, e a privacidade da escolha viraria premissa. Ele vem do Release público, igual para todos, com o SHA-256 na página da loja.

**"O túnel não vê o tráfego?"**
Não: no Tailscale Funnel, a conexão cifrada termina na nossa máquina; o provedor só repassa bytes cifrados. Com ngrok ou Cloudflare, que decifram, um provedor único veria compra e download juntos.
