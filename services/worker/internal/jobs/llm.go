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

const workerSystemPrompt = "Analise a vaga e responda em JSON estruturado seguindo estes criterios: vaga backend em Go ou Python, com beneficios explicitamente listados. Retorne apenas JSON com os campos decisao, titulo_vaga, empresa e justificativa."

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
		"%s\n\nResponda apenas em JSON valido com os campos decisao, titulo_vaga, empresa e justificativa.\n\nTitulo: %s\nEmpresa: %s\nLink: %s\nDescricao: %s",
		workerSystemPrompt,
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