# jobot

API em Go usando Echo, preparada para deploy exclusivo como Cloud Functions Gen 2 com uma estrutura simples, legivel e facil de manter.

## Como desenvolver localmente

```bash
go mod tidy
go build ./...
```

O deploy e orientado a Functions Framework no GCP. Nao existe mais um `main.go` para subir um servidor HTTP dedicado como processo da aplicacao.

### Rodar localmente e testar via HTTP

Para subir a funcao localmente com o Functions Framework:

```bash
FUNCTION_TARGET=Trigger LOCAL_ONLY=true go run ./cmd/local
```

No PowerShell:

```powershell
$env:FUNCTION_TARGET="Trigger"
$env:LOCAL_ONLY="true"
go run ./cmd/local
```

Depois disso, voce pode testar com Postman ou curl em `POST http://localhost:8080/`.

O entrypoint local recebe a chamada em `/` e a encaminha internamente para a rota `/trigger`.

## Rota disponivel

### `POST /trigger`

Retorna uma resposta JSON simples confirmando o acionamento da rota.

Exemplo de resposta:

```json
{
  "message": "trigger received"
}
```

## Middlewares aplicados

- `RequestID`: adiciona um identificador por requisicao.
- `RequestLogger`: registra metodo, rota, status, latencia e IP.
- `Recover`: evita queda do processo em caso de panic.
- `Secure`: aplica cabecalhos de seguranca basicos.
- `RemoveTrailingSlash`: normaliza URLs com barra final.

## Estrutura do projeto

```text
cmd/local
function.go
internal/http/app
internal/http/handlers
internal/http/routes
cloudbuild.function-gen2.yaml
```

## O que cada modulo faz

- `cmd/local`: runner local do Functions Framework para testes HTTP fora do GCP.
- `function.go`: entrypoint HTTP registrado no Functions Framework para Cloud Functions Gen 2.
- `internal/http/app`: composicao do handler Echo e middlewares da funcao.
- `internal/http/routes`: registro centralizado das rotas HTTP.
- `internal/http/handlers`: implementacao dos handlers de cada endpoint.

## Observacoes de arquitetura

O codigo foi separado por responsabilidade para facilitar manutencao, testes e expansao futura. Novas rotas podem ser adicionadas criando novos handlers e registrando-os em `internal/http/routes` sem acoplar regras de negocio ao entrypoint da funcao.

## Deploy no GCP Cloud Functions Gen 2

### O que foi ajustado

- O Echo e reutilizado como `http.Handler`, sem processo proprio escutando porta.
- O entrypoint HTTP exportado chama-se `Trigger` em `function.go`.
- O deploy e feito por pipeline usando `cloudbuild.function-gen2.yaml`.

### Deploy com Cloud Functions Gen 2

```bash
gcloud builds submit --config cloudbuild.function-gen2.yaml \
  --substitutions _FUNCTION_NAME=jobot-trigger,_REGION=us-central1,_RUNTIME=go125,_ENTRY_POINT=Trigger
```

### Observacao importante

No modelo Cloud Functions Gen 2, a funcao HTTP recebe todas as requisicoes no entrypoint configurado. Neste projeto, o entrypoint `Trigger` encaminha a requisicao para a stack Echo interna, preservando middlewares, logs e headers seguros.
