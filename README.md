# jobot

Monorepo em Go com dois servicos isolados por modulo:

- `services/function`: funcao HTTP responsavel pela captura inicial de vagas.
- `services/worker`: worker batch responsavel pela triagem e notificacao.

O repositorio usa Go Workspaces em `go.work` para desenvolvimento conjunto, mas cada servico tem seu proprio `go.mod`, dependencias e pipeline de deploy.

## Estrutura

```text
services/
  function/
    cmd/local/
    internal/http/
    internal/jobs/
    function.go
    go.mod
  worker/
    cmd/worker/
    internal/jobs/
    Dockerfile
    worker.go
    go.mod
deploy/
  function.cloudbuild.yaml
  worker.cloudbuild.yaml
go.work
```

## Ambiente local

Crie um arquivo `.env` na raiz do repositorio:

```dotenv
DATABASE_URL=postgresql://user:password@host/database?sslmode=require
LLM_API_URL=https://api.siliconflow.com/v1/chat/completions
LLM_API_KEY=your-llm-api-key
LLM_MODEL=deepseek-ai/DeepSeek-V3
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/your/webhook
```

Os dois servicos tentam carregar `.env` tanto do diretorio atual quanto da raiz do monorepo.

## Desenvolvimento

Para validar os dois modulos no workspace:

```bash
go build ./services/function/...
go build ./services/worker/...
```

### Function local

No PowerShell:

```powershell
$env:FUNCTION_TARGET="Trigger"
$env:LOCAL_ONLY="true"
go run ./services/function/cmd/local
```

A function recebe `POST http://localhost:8080/` e encaminha internamente para `/trigger`.

Payload opcional:

```json
{
  "terms": ["golang", "python backend"]
}
```

### Worker local

No PowerShell:

```powershell
go run ./services/worker/cmd/worker
```

O worker busca todas as vagas com status `PENDENTE`, processa cada item com jitter, chama a LLM configurada e envia aprovadas ao Discord.

A chamada da LLM segue o formato de `POST /v1/chat/completions` com `Authorization: Bearer`, `Content-Type: application/json`, `model` e um unico item em `messages` com `role: user`. A resposta e tratada no formato `choices[0].message.content`, equivalente ao fluxo `resp.raise_for_status(); resp.json()["choices"][0]["message"]["content"]`.

## Responsabilidades por servico

### Function

- Stack HTTP com Echo e middlewares.
- Endpoint `POST /trigger`.
- Scraper do LinkedIn Guest API.
- Gravacao inicial de vagas no PostgreSQL com deduplicacao.

### Worker

- Leitura batch de vagas pendentes.
- Enriquecimento por pagina publica de detalhe do LinkedIn.
- Integracao com LLM para decisao estruturada.
- Notificacao por Discord webhook.
- Atualizacao de status para `PROCESSADO` ou `ERRO`.

## Deploys isolados

### Function

```bash
gcloud builds submit --config deploy/function.cloudbuild.yaml .
```

Esse pipeline faz deploy da Cloud Function Gen 2 usando apenas `services/function` como source do servico.

### Worker

```bash
gcloud builds submit --config deploy/worker.cloudbuild.yaml .
```

Esse pipeline gera a imagem do worker a partir de `services/worker` e publica um Cloud Run Job separado.
