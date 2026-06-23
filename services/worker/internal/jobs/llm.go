package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var ErrMissingLLMAPIURL = errors.New("LLM_API_URL is not set")

const workerSystemPrompt = `
INSTRUCAO
Voce analisa vagas de tecnologia e decide se a vaga combina com o candidato descrito abaixo.

CANDIDATO_ALVO
- cargo: desenvolvedor backend pleno
- linguagens principais: Go e Python
- stack recorrente: Echo, FastAPI, Flask, RabbitMQ, Docker, PostgreSQL, Firebase e GCP
- experiencia relevante: ETL, integracao entre APIs e microsservicos, arquitetura orientada a eventos

REGRAS_DE_APROVACAO
- Retorne APROVADO somente se todos os requisitos obrigatorios forem atendidos.
- Retorne REJEITADO se qualquer requisito obrigatorio falhar ou se a descricao for insuficiente para comprovar os requisitos.

REQUISITOS_OBRIGATORIOS
1. O foco principal da vaga deve ser desenvolvimento backend em Go ou Python.

REGRAS_DE_REJEICAO
- Rejeite vagas cujo foco principal seja Java, .NET, PHP, frontend, mobile, data science.

COMO_AVALIAR
- Priorize o texto da descricao da vaga.
- Use titulo, empresa e link apenas como contexto complementar.
- Se houver conflito entre titulo e descricao, priorize a descricao.
- Nao invente informacoes ausentes.

FORMATO_DE_RESPOSTA
- Responda com JSON puro.
- Nao use markdown.
- Nao use bloco de codigo.
- Nao escreva nenhuma explicacao fora do JSON.
- O campo decisao deve ser texto, nunca boolean, numero ou objeto.
- Use exatamente um destes valores string em decisao: "APROVADO" ou "REJEITADO".

ESQUEMA_JSON
{
	"decisao": "APROVADO" | "REJEITADO",
	"titulo_vaga": "string",
	"empresa": "string",
	"justificativa": "string curta e objetiva"
}
`

const defaultLLMAPIURL = "https://api.siliconflow.com/v1/chat/completions"
const defaultLLMModel = "deepseek-ai/DeepSeek-V3"

type LLMClient interface {
	ReviewJob(ctx context.Context, details JobDetails) (JobReview, error)
}

type HTTPStructuredLLMClient struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
	model      string
}

type llmRequest struct {
	Model    string       `json:"model,omitempty"`
	Messages []llmMessage `json:"messages"`
}

type llmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type llmWrappedResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type rawJobReview struct {
	Decision      any `json:"decisao"`
	JobTitle      any `json:"titulo_vaga"`
	Company       any `json:"empresa"`
	Justification any `json:"justificativa"`
}

func NewHTTPStructuredLLMClient() *HTTPStructuredLLMClient {
	return &HTTPStructuredLLMClient{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		endpoint:   fallbackEnv("LLM_API_URL", defaultLLMAPIURL),
		apiKey:     strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		model:      fallbackEnv("LLM_MODEL", defaultLLMModel),
	}
}

func (c *HTTPStructuredLLMClient) ReviewJob(ctx context.Context, details JobDetails) (JobReview, error) {
	if c.endpoint == "" {
		return JobReview{}, ErrMissingLLMAPIURL
	}

	payload := llmRequest{
		Model: c.model,
		Messages: []llmMessage{
			{Role: "system", Content: workerSystemPrompt},
			{Role: "user", Content: buildJobReviewPrompt(details)},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return JobReview{}, fmt.Errorf("marshal llm request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return JobReview{}, fmt.Errorf("build llm request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return JobReview{}, fmt.Errorf("perform llm request: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return JobReview{}, fmt.Errorf("read llm response: %w", err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return JobReview{}, fmt.Errorf("llm request failed with status %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	review, err := parseLLMReview(responseBody)
	if err != nil {
		return JobReview{}, err
	}

	review.Decision = strings.ToUpper(strings.TrimSpace(review.Decision))
	review.JobTitle = strings.TrimSpace(review.JobTitle)
	review.Company = strings.TrimSpace(review.Company)
	review.Justification = strings.TrimSpace(review.Justification)

	return review, nil
}

func parseLLMReview(responseBody []byte) (JobReview, error) {
	var wrapped llmWrappedResponse
	if err := json.Unmarshal(responseBody, &wrapped); err != nil {
		return JobReview{}, fmt.Errorf("decode llm response: %w", err)
	}

	if len(wrapped.Choices) == 0 {
		return JobReview{}, errors.New("llm response does not contain choices")
	}

	content := sanitizeLLMContent(wrapped.Choices[0].Message.Content)
	if content == "" {
		return JobReview{}, errors.New("llm response choice content is empty")
	}

	var rawReview rawJobReview
	if err := json.Unmarshal([]byte(content), &rawReview); err != nil {
		return JobReview{}, fmt.Errorf("decode llm structured content: %w", err)
	}

	review, err := normalizeRawJobReview(rawReview)
	if err != nil {
		return JobReview{}, err
	}

	return review, nil
}

func sanitizeLLMContent(content string) string {
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end >= start {
		return strings.TrimSpace(trimmed[start : end+1])
	}

	return trimmed
}

func buildJobReviewPrompt(details JobDetails) string {
	return fmt.Sprintf(
		"Analise a vaga abaixo e responda somente com JSON valido.\n\nDADOS_DA_VAGA\nTitulo: %s\nEmpresa: %s\nLink: %s\nDescricao:\n%s\n\nLEMBRETE_FINAL\nO campo decisao deve ser string com valor APROVADO ou REJEITADO. Nao use true, false, 0, 1 nem texto fora do JSON.",
		details.Title,
		details.Company,
		details.ApplicationLink,
		details.DescriptionText,
	)
}

func fallbackEnv(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}

	return value
}

func normalizeRawJobReview(raw rawJobReview) (JobReview, error) {
	decision, err := normalizeDecision(raw.Decision)
	if err != nil {
		return JobReview{}, err
	}

	return JobReview{
		Decision:      decision,
		JobTitle:      stringifyJSONValue(raw.JobTitle),
		Company:       stringifyJSONValue(raw.Company),
		Justification: stringifyJSONValue(raw.Justification),
	}, nil
}

func normalizeDecision(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		decision := strings.ToUpper(strings.TrimSpace(typed))
		switch decision {
		case "APROVADO", "REJEITADO":
			return decision, nil
		case "APROVAR", "TRUE", "ACEITO", "VALIDO", "VÁLIDO", "SIM":
			return "APROVADO", nil
		case "FALSE", "INVALIDO", "INVÁLIDO", "INVALID", "REPROVADO", "NEGADO", "NAO", "NÃO", "NAO APROVADO", "NÃO APROVADO":
			return "REJEITADO", nil
		default:
			return "REJEITADO", nil
		}
	case bool:
		if typed {
			return "APROVADO", nil
		}
		return "REJEITADO", nil
	case float64:
		if typed != 0 {
			return "APROVADO", nil
		}
		return "REJEITADO", nil
	case nil:
		return "", errors.New("llm response is missing decisao")
	default:
		return "", fmt.Errorf("unsupported decisao type %T", value)
	}
}

func stringifyJSONValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}