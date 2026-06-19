package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

var ErrMissingDiscordWebhookURL = errors.New("DISCORD_WEBHOOK_URL is not set")

type DiscordNotifier struct {
	httpClient *http.Client
	webhookURL string
}

type discordWebhookPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string              `json:"title"`
	URL         string              `json:"url,omitempty"`
	Description string              `json:"description"`
	Color       int                 `json:"color"`
	Fields      []discordEmbedField `json:"fields,omitempty"`
}

type discordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

func NewDiscordNotifier() *DiscordNotifier {
	return &DiscordNotifier{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		webhookURL: strings.TrimSpace(os.Getenv("DISCORD_WEBHOOK_URL")),
	}
}

func (n *DiscordNotifier) SendApprovedJob(ctx context.Context, jobID string, review JobReview, applicationLink string) error {
	if n.webhookURL == "" {
		return ErrMissingDiscordWebhookURL
	}

	payload := discordWebhookPayload{
		Embeds: []discordEmbed{{
			Title:       fallbackString(review.JobTitle, "Vaga aprovada"),
			URL:         applicationLink,
			Description: fallbackString(review.Justification, "Vaga aprovada pela triagem automatica."),
			Color:       3066993,
			Fields: []discordEmbedField{
				{Name: "Empresa", Value: fallbackString(review.Company, "Nao informada"), Inline: true},
				{Name: "ID da vaga", Value: jobID, Inline: true},
				{Name: "Link de candidatura", Value: applicationLink, Inline: false},
			},
		}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build discord webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := n.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("perform discord webhook request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("discord webhook returned status %d", response.StatusCode)
	}

	return nil
}