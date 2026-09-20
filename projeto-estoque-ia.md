# Sistema de Controle de Estoque com IA — Visão Geral do Projeto

## O que é

Um sistema de **controle de estoque de loja**, motivado por um problema real: o movimento da loja é grande, então não dá tempo de anotar manualmente cada produto que entra e sai. A ideia é usar a câmera do **celular** para agilizar isso — em vez de digitar, o funcionário tira uma foto do produto, o sistema identifica o que é, e ele só confirma a quantidade.

## Como vai funcionar (arquitetura)

Três partes, cada uma numa linguagem diferente, de acordo com o que cada uma faz melhor:

```
[Celular - PWA em JS/TS]  →  foto  →  [Backend em Go]  →  [Serviço de IA em Python]
                                              ↓
                                     [Banco de dados de estoque]
```

1. **App no celular (PWA, JavaScript/TypeScript)**
   Tela simples onde o funcionário tira a foto do produto e confirma a quantidade com poucos toques. Ainda não iniciado.

2. **Backend em Go**
   Recebe a foto/dados do celular, chama o serviço de IA para identificar o produto, registra a movimentação (entrada/saída) no banco, e serve como a "espinha dorsal" do sistema. **Parte em construção atualmente.**

3. **Serviço de reconhecimento de imagem em Python**
   Vai rodar um modelo de IA (provavelmente YOLO) treinado para reconhecer os produtos específicos da loja. Ainda não iniciado.

## Decisões importantes já tomadas

- **Não vamos tentar "contar quantidade" só pela foto.** Isso é impreciso — a IA identifica *qual* produto é; a quantidade é confirmada por um humano com poucos toques.
- **O sistema vai funcionar com internet externa** (Wi-Fi ou dados móveis) — não precisa ser tudo local/offline. Isso simplifica bastante, porque dá pra hospedar backend e IA na nuvem.
- Avaliamos código de barras como alternativa/complemento mais confiável, mas o foco definido foi seguir com reconhecimento por foto mesmo.

## Progresso no backend (Go)

Construindo aos poucos, com foco em aprendizado (início sem experiência prévia em Go/APIs).

- [x] Setup do projeto (`go mod init`)
- [x] Servidor HTTP mínimo (`/health`)
- [x] Rota GET devolvendo JSON (`/produto`)
- [x] Rota POST recebendo JSON (`/movimentacao`), guardando em memória
- [ ] **Próximo passo:** trocar a memória por um banco de dados de verdade (SQLite), para os dados não sumirem quando o servidor reinicia
- [ ] Serviço de reconhecimento de imagem em Python (IA)
- [ ] App do celular (PWA em JS/TS)

## Ambiente de desenvolvimento

- Sistema operacional: Windows
- Editor: VS Code com extensão oficial Go instalada
- Go e ferramentas auxiliares (gopls, dlv) já configurados
