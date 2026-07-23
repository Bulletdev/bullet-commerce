# PRD — Assistente de Compras & Suporte por IA (bullet-commerce)

**Data:** 2026-07-22 · **Status:** proposta (V1 / PoC) · **Plano:** ver `DEVDOCS/IMPLEMENTATION-PLAN-ai-assistant.md`
**Produto:** assistente conversacional embutido na loja (front clubedojava) exposto por endpoint no bullet-commerce, com RAG sobre o catálogo + tool-use contra a API (dados do usuário autenticado), powered por Claude. Mercado BR (pt-BR, LGPD). Capacidade do perfil **Growth+** (ver `PRD-flamingo-gap-analysis.md`).

> Documento estruturado no framework de PRD de IA em 5 seções: Visão & Objetivos · Personas & Casos de Uso · Requisitos Funcionais · Requisitos Não Funcionais · Escopo & Restrições.

---

# 1. Visão Geral e Objetivos

## 1.1 Declaração do Problema

**A dor do cliente (comprador BR).** Numa loja online brasileira, o cliente trava em pontos previsíveis e o site raramente responde na linguagem dele:

- **Dúvida de produto/tamanho/compatibilidade:** "essa caneca é de 300ml ou 400ml?", "essa camiseta veste largo? sou 1,80m e 85kg, qual tamanho?" — a ficha técnica existe mas está espalhada e não responde a pergunta específica.
- **Estoque por variante:** "tem na cor preta tamanho G?" — a informação existe no sistema (estoque por variante com Reserve/Claim/Release), mas o cliente precisa clicar variante por variante para descobrir.
- **Status e rastreio de pedido:** "meu pedido saiu?", "paguei o PIX e não atualizou" — o cliente não entende a diferença entre "pedido confirmado" e "pagamento aprovado".
- **Frete e prazo por CEP:** "chega até dia X no meu CEP?" — decisão de compra que depende de cálculo por CEP.
- **Troca/devolução (pós-venda):** "comprei errado, como troco?", "veio com defeito" — o cliente não sabe o direito dele (arrependimento em 7 dias / CDC) nem o passo a passo da loja.
- **Checkout PIX:** "gerei o PIX e sumiu", "quanto tempo tenho pra pagar?" — dúvida no momento mais crítico do funil, onde qualquer fricção vira carrinho abandonado.

**A dor do lojista (multi-perfil Starter→Enterprise).** Cada dúvida vira custo/tempo de atendimento humano (o lojista Starter/Growth muitas vezes responde ele mesmo, fora do horário, as mesmas 20 perguntas), **abandono de carrinho por dúvida não resolvida**, e **tickets repetitivos** ("cadê meu pedido", "como troco") que afogam os casos que realmente precisam de humano.

O problema central: **a informação já existe na plataforma bullet-commerce** (catálogo, estoque por variante, pedidos, status/payment_status, dados de PIX), mas não está acessível ao cliente em **linguagem natural, no momento e no contexto certos**.

## 1.2 Proposta de Valor — Por que IA e não lógica determinística

FAQ estático, busca por palavra-chave ou árvore de decisão (chatbot de botões) falham exatamente onde o cliente BR mais precisa:

1. **Linguagem natural e variação infinita.** "veste apertado?", "é justa?", "encolhe?", "serve pra gordinho?" são a mesma dúvida de caimento — um FAQ fixo nunca cobre as M formas de perguntar. O LLM entende intenção, gíria e erro de português.
2. **Contexto do pedido e composição de resposta.** "cadê meu pedido e quando chega?" exige identificar o pedido do usuário, ler `status`+`payment_status`, cruzar com rastreio e prazo por CEP, e responder numa frase humana — uma **resposta composta a partir de múltiplas fontes vivas**. Com **tool-use**, o assistente chama a API, obtém o dado real e compõe.
3. **RAG mantém a resposta fiel ao catálogo real.** O modelo responde **grounded** em trechos reais da ficha/política; catálogo mudou de preço, a resposta muda junto, sem retreinar nada.
4. **Escala por perfil com custo controlado via seleção de modelo.** Haiku 4.5 para alto volume simples, Sonnet 5 para dúvidas complexas, Opus 4.8 para casos raros — viável economicamente até no Starter.

**Onde IA NÃO é a resposta (explícito):**
- **Nunca decide/executa ações irreversíveis ou financeiras sozinha:** não emite reembolso, não cancela pedido, não altera endereço, não aplica cupom, não gera NF-e. Pode *orientar* e *preparar*; a execução passa por fluxo determinístico e/ou confirmação humana.
- **Cálculo de preço, total, frete, estoque e prazo é sempre determinístico** — vem de tool-call à API, nunca do "achismo" do modelo. O LLM só verbaliza o número que a ferramenta retornou.
- **Regras legais e fiscais (CDC, LGPD, NF-e, arrependimento) não são "geradas"** — são recuperadas de fonte oficial da loja via RAG.
- **Autenticação e autorização não são responsabilidade do modelo** — quem o usuário é, e quais pedidos pode ver, é decidido pela API com o token; o modelo nunca "confia" no que o cliente disser sobre identidade.

## 1.3 Métricas de Sucesso

| # | Métrica | Meta inicial | Como medir |
|---|---------|--------------|------------|
| M1 | **Containment (resolução sem humano)** | ≥ 55% das conversas | % encerradas sem handoff e sem reabertura do mesmo assunto em 48h |
| M2 | **Groundedness / precisão factual** | ≥ 90% (0% em dado financeiro é **gate**) | Amostragem semanal: toda afirmação de preço/estoque/status/prazo bate com a tool ou o trecho de RAG citado |
| M3 | **Redução de tickets** | −30% em "status" e "como trocar" | Volume por categoria pré vs. pós, normalizado por nº de pedidos |
| M4 | **CSAT** | ≥ 4,2/5 (ou ≥ 80% 👍) | Micro-pesquisa ao fim da conversa, segmentada por intenção |
| M5 | **Conversão / abandono** | +2 p.p. de conversão; −15% de abandono no checkout PIX entre quem interagiu | A/B (com vs. sem assistente) no funil, atribuído por sessão |
| M6 | **Latência p95** | ≤ 3 s primeira resposta; ≤ 7 s p95 com 1 tool-call (ver §4.1) | Tempo até primeira token/resposta, segmentado por modelo e caminho |
| M7 | **Custo por conversa** | ≤ teto por plano | Tokens in/out × preço, com prompt caching; monitorar mix de roteamento |
| M8 | **Taxa de recusa saudável** | monitorada (não maximizada) | % de recusas/encaminhamentos; recusar demais indica RAG/tools fracos |

**M2 (groundedness) e M6 (latência) são restrições rígidas**; M1, M3 e M5 são os indicadores de valor de negócio.

---

# 2. Personas e Casos de Uso

## 2.1 Público-alvo

**Primárias (usuárias diretas):**
- **Bruna — compradora nova.** Veio de anúncio, não conhece a marca, dúvida de tamanho e "isso chega mesmo?". Sensível a fricção; abandona fácil.
- **Rafael — comprador recorrente.** Tem conta e histórico; quer velocidade ("cadê o que comprei semana passada"). Autenticado — o assistente consulta os pedidos dele.
- **Camila — compradora com problema pós-venda.** Produto errado/defeito ou arrependimento (7 dias/CDC). Estado emocional elevado; precisa de clareza de direito + passo a passo + handoff humano rápido.

**Secundária:** **Diego — lojista/admin (Growth/Scale).** Não conversa com o bot; configura políticas (troca, frete), define o tom da marca, consome relatórios (containment, dúvidas top, buracos de catálogo).

**Impactados indiretos:** **equipe de suporte humano** — deixa de responder repetição e recebe conversas já contextualizadas. Risco a gerenciar: percepção de substituição — posicionar como filtro, não substituto.

## 2.2 Jornada do Usuário

### A — Dúvida de produto pré-compra (Bruna)
1. Na página de uma camiseta: *"veste largo? tenho 1,80 e uso M."*
2. **IA entra** interpretando intenção = caimento. Faz **RAG** na ficha (tabela de medidas) — não inventa.
3. Follow-up: *"quer na cor preta? qual tamanho verifico?"* → **tool `check_variant_stock`**.
4. Responde composto e grounded: *"Nessa modelagem o M veste justo; pra 1,80m eu iria de G. Temos G preto em estoque. Deixo separado no carrinho?"* — cita a fonte.
5. **Handoff:** nenhum; adicionar ao carrinho é confirmado pela Bruna (a IA prepara, não executa).

### B — Status / rastreio (Rafael, autenticado)
1. *"cadê meu pedido?"* → **tool `list_my_orders`** (API resolve identidade pelo token).
2. Lê `status`+`payment_status` e o `tracking_number` (via `get_my_order_status`); prazo por CEP via `quote_shipping`. Rastreamento ao vivo na transportadora = **V2**.
3. *"Pedido #1234 foi enviado ontem, código BR123…, previsão até 24/07 no seu CEP."* — todo número vem de tool.
4. **Handoff** só em anomalia (enviado há 15 dias sem movimento) → humano com o pedido já identificado.

### C — Troca / devolução (Camila) · **V2 — fora da V1**
1. *"chegou rasgado, quero trocar."* → identifica pedido, confirma item e data (arrependimento/CDC).
2. **RAG na política de troca da loja** — regras reais, não improvisa direito.
3. Explica direito + passo a passo e **prepara** o fluxo. **Não executa** reembolso nem gera etiqueta.
4. **Handoff obrigatório** ao time/RMA com pedido, item e motivo estruturados. Tom empático (roteia Sonnet/Opus).

### D — Ajuda no checkout PIX (leitura de status = V1; **regenerar PIX = V2**)
1. *"gerei o PIX e não sei se pagou"* → **tool de status de pagamento** (ProPay/OpenPix/Efí) para o pedido em aberto.
2. Pendente: informa expiração do QR + *"assim que o banco confirmar, o pedido confirma automaticamente — não precisa comprovante"*. Expirado: oferece regenerar (confirmado pelo usuário).
3. Nunca afirma "seu pagamento caiu" sem `payment_status` aprovado.
4. **Handoff** se PSP aprovou mas o pedido não confirmou (divergência) → humano com os IDs.

## 2.3 Cenários de Fracasso e Mitigações

| Falha | Exemplo | Mitigação |
|-------|---------|-----------|
| Inventar preço/desconto | "sai por R$79" sendo R$99 | Preço só via tool; proibido afirmar valor sem retorno de ferramenta. Tool falhou → mostra o valor da página |
| Inventar estoque | "tem sim!" com variante zerada | Disponibilidade só via `check_variant_stock`; estoque volátil (Reserve/Claim/Release) → "confirmo no carrinho" |
| Inventar prazo/frete | "chega amanhã" sem calcular | Só via tool por CEP; sem CEP → pede o CEP, não estima |
| Confundir pedido de outro usuário | vaza dado pessoal (LGPD) | Autorização é da API pelo token, não do modelo; tools de pedido só no escopo do usuário; RLS no banco |
| Prometer o que a loja não faz | "devolve em 90 dias" | Políticas via RAG com citação; sem trecho → recusa e encaminha |
| Alucinação factual | característica que o produto não tem | Resposta grounded no trecho de RAG; citar fonte; sem dado → "não tenho essa informação" + humano |
| Erro emocional pós-venda | robótico com cliente irritado | Intenção sensível → modelo mais capaz + tom empático + handoff sempre visível |
| Divergência de pagamento | "não pagou" mas PSP aprovou | Sempre ler `payment_status` real; em divergência não decide → escala com IDs |

**Princípios transversais:** (1) grounding obrigatório; (2) recusa honesta; (3) fallback humano a um clique; (4) nunca afirmar preço/estoque/prazo/status sem tool-call; (5) a IA orienta e prepara, mas não executa ações financeiras/irreversíveis.

---

# 3. Requisitos Funcionais

## 3.1 Input e Output

**Input (V1):** texto livre em pt-BR (gíria, erro, abreviação — "cade meu pedido", "qnd chega", "tem no M?"); contexto de sessão implícito (JWT do usuário, `store_id`, histórico da conversa). **Fora do V1:** imagem de produto e áudio — a arquitetura de tool-use permite adicioná-los depois sem reescrever o núcleo.

**Output:** resposta híbrida = **texto conversacional** + **elementos estruturados** que o front React renderiza como componentes nativos (via contrato JSON estável do BFF):

| Elemento | Quando | Payload |
|---|---|---|
| **Card de produto** | Busca/recomendação | `product_id`, `variant_id`, título, `price_cents` (R$ no front), imagem, disponibilidade |
| **Link de pedido** | Status/rastreio | `order_id`, deep-link, `status` + `payment_status` |
| **QR / copia-e-cola PIX** | Pagamento pendente | `qr_code`, `qr_code_text`, `expires_at` — **nunca gerado pela IA; repassado do provedor via API** |
| **Botão de ação** | Próximo passo | "Ver carrinho", "Rastrear", "Falar com atendente" — dispara rota do front |

Respostas concisas (1–3 parágrafos), pt-BR informal-profissional, preço sempre formatado de `price_cents/100`. Toda afirmação factual ancorada em tool (§3.3). Default **Claude Haiku 4.5** (`claude-haiku-4-5`); casos difíceis escalam **Sonnet 5** (`claude-sonnet-5`).

## 3.2 Ferramentas (Tool-use) — o coração funcional

O assistente **não responde dado de negócio de memória** — chama tools contra a API e responde grounded no retorno. **Regra transversal de autorização:** toda tool de dados de usuário opera **exclusivamente sob o escopo do JWT** — `user_id`/`store_id` derivados do token no BFF, **nunca** de argumento que o modelo preencha. Tentativa de acessar dado de terceiro é rejeitada na camada de autorização, não no prompt.

**Read-only (executam automaticamente):**

| Tool | Propósito | Autorização |
|---|---|---|
| `search_catalog(query, filtros?)` | Buscar catálogo (RAG + busca facetada) | Público, escopado por `store_id` |
| `get_product(product_id)` | Detalhar produto + variantes | Público, `store_id` |
| `check_variant_stock(variant_id)` | Estoque real (Reserve/Claim/Release) | Público, `store_id` |
| `quote_shipping(cep, itens?)` | Frete + prazo | Público (CEP não é dado sensível de conta) |
| `get_my_order_status(order_id)` | Status de um pedido | **Só se pertence ao `user_id` do JWT; senão `not_found`** (não vaza existência) |
| `list_my_orders()` | Pedidos do usuário | **Restrita ao autenticado** |

**Mutantes de estado — `V2, FORA DA V1` (exigem confirmação humana explícita):**

| Tool | Propósito | Regra |
|---|---|---|
| `add_to_cart(variant_id, qty)` | Adicionar ao carrinho | Requer estoque confirmado antes; confirma intenção |
| `initiate_return(order_id, motivo)` | Abrir troca/devolução | Só após confirmação; escopado ao JWT; não decide elegibilidade — segue política ou escala |
| `create_pix_charge(order_id)` | Gerar cobrança PIX | **Nunca dispara pagamento sozinho.** Só gera QR após confirmação; a IA solicita, o provedor processa |

**Princípio:** read-only roda direto; qualquer criação/alteração/cobrança passa por confirmação humana (botão de ação). O modelo propõe; o usuário confirma; o BFF executa.

## 3.3 Grounding / RAG

Catálogo indexado com embeddings (`pgvector`) e/ou a busca facetada do roadmap (híbrido semântico + facetado), escopado por `store_id`.

> **Regra dura (inegociável):** a IA **nunca** afirma preço, estoque, prazo, status de pedido ou de pagamento sem uma tool-call correspondente na mesma resposta.

Preço → `search_catalog`/`get_product`. Estoque → `check_variant_stock`. Prazo/frete → `quote_shipping`. Status → `get_my_order_status`/`list_my_orders`. O RAG serve para **encontrar** o produto e responder sobre características/descrição; **valores factuais e transacionais são sempre do dado real da tool**, nunca do texto indexado (que pode estar defasado). Sem dado → diz que não tem, não estima.

## 3.4 Tom e Personalidade

Atendente prestativo, direto e honesto de loja BR. pt-BR informal-profissional.
**Do's:** conciso; honesto sobre limites; transparente que é IA; confirmar antes de ação que mexe em pedido/carrinho/pagamento; reclamação = empatia primeiro, solução depois.
**Don'ts:** nunca pushy (sem upsell repetido/pressão); não inventar dado; não prometer o que não pode cumprir; não fingir ser humano; sem emoji em excesso em reclamação.
**Cliente irritado:** baixar o tom, validar o sentimento, não culpar o cliente, buscar o dado real via tool e escalar rápido se fora do escopo — nunca script genérico sem checar o pedido concreto.

## 3.5 Regras de Negócio e Fallbacks

**Contingência** (não sabe / sem dado / fora de escopo): mensagem padrão honesta + oferta de handoff → *"Isso eu não consigo resolver por aqui. Quer que eu abra um chamado pra equipe? O atendimento humano funciona [horário]."* O handoff cria um ticket com o contexto já coletado (pedido, sintoma). Horário conforme a loja (multi-loja).

**Sempre escalar para humano:** problema de pagamento (cobrança duplicada, PIX não reconhecido, estorno, divergência de `payment_status`); dado sensível (alteração de CPF, dados bancários, exclusão de conta/LGPD); exceção comercial (desconto não tabelado, cancelamento fora de política); sinal de fraude/abuso.

**Regras rígidas:** não dar aconselhamento fora do escopo da loja (jurídico/médico/financeiro); não prometer descontos não autorizados; não processar pagamento sem confirmação explícita (a IA apresenta o meio; o usuário confirma; o provedor processa); LGPD — só lê/exibe dado do próprio titular; solicitação de tratamento/exclusão vai ao fluxo humano/DPO.

---

# 4. Requisitos Não Funcionais (Technical Boundaries)

## 4.1 Latência

Cada resposta = inferência do modelo (TTFT + geração) + round-trips de tool. **Cada round-trip embute uma passada completa de inferência** (emite `tool_use` → executa no BFF 50–200 ms → nova inferência sobre o `tool_result`), não só o tempo do banco.

Alvos (Haiku 4.5 default, cache quente):

| Interação | TTFT p50 | TTFT p95 | Completa p50 | Completa p95 |
|---|---|---|---|---|
| Simples (sem tool) | ≤ 0,6 s | ≤ 1,5 s | ≤ 2,0 s | ≤ 4,0 s |
| 1 tool-call | ≤ 0,8 s | ≤ 1,8 s | ≤ 3,5 s | ≤ 7,0 s |
| Multi-turn / 2–3 tools | ≤ 1,0 s | ≤ 2,2 s | ≤ 6,0 s | ≤ 12,0 s |

**Streaming (SSE)** obrigatório — ganho de **percepção** (primeiro token ~0,5 s, lê enquanto gera). Exibir estado intermediário ("consultando estoque…") entre round-trips. **Nº de tools é o principal fator de latência** (mais que o tamanho do prompt cacheado): preferir tool-use **paralelo** para consultas independentes; manter tools do BFF com p99 < 200 ms; limitar o loop (`max_iterations = 5`).

## 4.2 Modelo e Custo

| Modelo | Model ID | Input $/1M | Output $/1M | Contexto | Papel |
|---|---|---|---|---|---|
| Haiku 4.5 | `claude-haiku-4-5` | US$ 1,00 | US$ 5,00 | 200K | **Default** — chat, FAQ, lookups |
| Sonnet 5 | `claude-sonnet-5` | US$ 3,00¹ | US$ 15,00¹ | 1M | Escalonamento — complexo/multi-step |
| Opus 4.8 | `claude-opus-4-8` | US$ 5,00 | US$ 25,00 | 1M | Exceção — raciocínio pesado |

¹ Sonnet 5 tem promo US$ 2/US$ 10 por MTok até 2026-08-31; projeções usam preço-cheio por conservadorismo.

**Roteamento:** maioria em Haiku 4.5 (melhor custo/latência); escala a Sonnet 5 quando (a) raciocínio multi-step/comparação, (b) baixa confiança do Haiku, (c) problema de pedido multi-condição; Opus só em casos raros de alto valor+dificuldade. Não trocar de modelo no meio da conversa sem necessidade (o cache é por modelo).

**Prompt caching:** prefixo estável (system prompt + tools + catálogo/políticas RAG) com `cache_control`. `cache_read` ≈ 0,1× input, `cache_write` = 1,25× (5 min) / 2× (1 h). Com 2+ requisições sobre o mesmo prefixo (toda conversa multi-turn) já se paga; corta input em ~2,5×. TTL default 5 min (janela da conversa). Prefixo cacheável mínimo em Haiku 4.5 = **4096 tokens** (Sonnet 5 = 2048).

**Custo por conversa** (típica = 5 turnos; prefixo cacheado ~12k tokens):
- **Haiku 4.5 ≈ US$ 0,038** (~US$ 0,04): input ~31k tokens-equiv × US$1 + output 1,4k × US$5.
- **Sonnet 5 ≈ US$ 0,11** (preço-cheio).
- **Misto 80/20 ≈ US$ 0,054/conversa.**

**Projeção mensal** (2.000 conversas/dia → 60k/mês): só Haiku ~US$ 2.400; misto 80/20 ~US$ 3.240; **sem caching ~US$ 5.400** (caching corta ~40% e melhora TTFT). Escala linear com volume.

## 4.3 Segurança, Privacidade e Compliance (LGPD)

**Minimização (obrigatória):** o modelo recebe só o necessário (catálogo, políticas, histórico, resultados de tools já escopados). **Nunca enviar ao modelo** CPF, dados de cartão/pagamento, tokens — se a tool precisa, manipula **dentro do BFF** e retorna só o resultado ("pagamento aprovado", "pedido #123 entregue"). CEP entra só para frete.

**Trânsito de dados:** requisições vão à Anthropic API sob política de **não-treinamento** e retenção mínima; documentar no DPA e no aviso de privacidade. Nada persistido pela camada de IA além do necessário.

**Isolamento por usuário (controle central):** tools executadas pelo BFF **só no escopo do JWT** — `user_id`/`store_id` do token validado, nunca de argumento do modelo. Se o modelo pedir pedido de outro usuário, a tool ignora o ID do prompt e consulta com o `user_id` do JWT (RLS reforça no banco). O modelo não acessa o banco direto.

**Prompt injection / exfiltração:** conteúdo não-confiável (descrições, mensagens) pode conter instruções maliciosas. Mitigações: tools escopadas ao JWT (exfiltração cross-tenant impossível); instruções de operador só pelo canal de sistema; guardrails de saída (não vazar dado de outro cliente, não expor system prompt); limitar o loop e auditar sequências anômalas.

**Logs:** retenção mínima com anonimização/pseudonimização (remover CPF/contatos); janela explícita; **direito de exclusão LGPD** (apagar logs junto aos dados do usuário). Logs servem depuração/melhoria, não treinamento de terceiros.

**Transparência:** informar de forma clara que é uma **IA** e disponibilizar o aviso de tratamento de dados + caminho de escalonamento humano.

---

# 5. Escopo e Restrições

## 5.1 O que entra na V1.0 (PoC validável)

Um único caso de uso central: **Assistente de Pré-venda + Status de Pedido**. O corte prova a tese técnica e de valor (RAG sobre catálogo + tool-use read-only + loop conversacional Claude, dentro de ports & adapters) com o menor risco e o menor tempo até o primeiro sinal mensurável.

1. **Dúvidas de pré-venda via RAG** — produtos, variantes, estoque, preço e frete, ancorado no catálogo real. Ex.: *"tem G em preto?"*, *"quanto sai o frete pra 30140-071?"*.
2. **Tools read-only** (allowlist estrita): `search_catalog`, `get_product`, `check_variant_stock`, `quote_shipping` — reusam `internal/variants` e `internal/shipping`, sem escrita.
3. **Status do próprio pedido** — usuário autenticado consulta `status`+`payment_status` via `get_my_order_status`, isolado pelo JWT.
4. **Fallback para humano** — baixa confiança / frustração / fora de escopo → encerra com handoff claro.
5. **Streaming (SSE)** + **rate limit** por usuário.
6. **Feature flag** `FEATURE_AI_ASSISTANT` desligada por padrão; só perfil **Growth+**.

**Por que só pré-venda + status:** valida a PoC rápido (par de intenções mais frequente e de maior valor, sem mutação sensível); read-only elimina a classe de risco mais cara (nada irreversível); cabe na arquitetura atual sem dependência pesada (RAG começa em `tsvector`, tools são adapters finos sobre repos já verdes); é medível desde o dia 1 (containment, groundedness, custo/conversa).

## 5.2 O que fica de fora (V1)

| Fora da V1 | Por quê |
|---|---|
| Processar pagamento / reembolso automático | Mutação financeira irreversível; exige humano-in-the-loop e auditoria |
| Abrir/aprovar troca sem humano | Impacto operacional/estoque; V2 com aprovação humana |
| Recomendação personalizada por ML | Exige modelo/feature store próprios; outro projeto |
| Busca por imagem/voz (multimodal) | Amplia superfície e custo sem validar o caso textual central |
| Multi-idioma | Mercado BR/pt-BR; multiplica evals sem retorno na PoC |
| Ação que mute estado sem confirmação | Viola a reversibilidade da V1 |
| WhatsApp / canais externos | Cada canal é adaptador de entrega + conformidade próprios; V1 é só o widget web |
| Geração de conteúdo de marketing | Fora do domínio de suporte/venda assistida |
| Memória cross-sessão / perfil aprendido | Persistência levanta questões de LGPD que a PoC não precisa resolver |
| Autonomia sem teto de custo/turnos | Sem `AI_MAX_TOOL_ITERS` + guarda de custo acumulado a PoC vira risco financeiro (o default Haiku 4.5 não tem `task_budget`) |

---

*Plano de implementação em 9 work items: `DEVDOCS/IMPLEMENTATION-PLAN-ai-assistant.md`.*
