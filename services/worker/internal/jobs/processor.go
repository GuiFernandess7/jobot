package jobs

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const linkedInJobPostingURL = "https://www.linkedin.com/jobs-guest/jobs/api/jobPosting/%s"

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)
var jobDescriptionPattern = regexp.MustCompile(`(?s)<div[^>]*class="[^"]*show-more-less-html__markup[^"]*"[^>]*>(.*?)</div>`)
var jobTitlePattern = regexp.MustCompile(`(?s)<h2[^>]*class="[^"]*top-card-layout__title[^"]*"[^>]*>(.*?)</h2>`)
var companyNamePattern = regexp.MustCompile(`(?s)<a[^>]*class="[^"]*topcard__org-name-link[^"]*"[^>]*>(.*?)</a>|<span[^>]*class="[^"]*topcard__flavor[^"]*"[^>]*>(.*?)</span>`)

type JobReview struct {
	Decision      string `json:"decisao"`
	JobTitle      string `json:"titulo_vaga"`
	Company       string `json:"empresa"`
	Justification string `json:"justificativa"`
}

type JobDetails struct {
	ID              string
	Title           string
	Company         string
	DescriptionText string
	ApplicationLink string
}

type Processor struct {
	logger   *slog.Logger
	store    *Store
	llm      LLMClient
	notifier *DiscordNotifier
	rand     *rand.Rand
	client   *http.Client
}

func NewProcessor(logger *slog.Logger) (*Processor, error) {
	store, err := NewStore(context.Background())
	if err != nil {
		return nil, fmt.Errorf("create worker store: %w", err)
	}

	return &Processor{
		logger:   logger,
		store:    store,
		llm:      NewHTTPStructuredLLMClient(),
		notifier: NewDiscordNotifier(),
		rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (p *Processor) Close() {
	if p == nil || p.store == nil {
		return
	}

	p.store.Close()
}

func (p *Processor) Run(ctx context.Context) error {
	p.logger.Info("worker started")
	defer p.logger.Info("worker finished")

	pendingJobs, err := p.store.FetchPendingJobs(ctx)
	if err != nil {
		return err
	}

	p.logger.Info("pending jobs loaded", "count", len(pendingJobs))
	for index, pendingJob := range pendingJobs {
		if err := p.processJob(ctx, pendingJob); err != nil {
			p.logger.Error("job processing failed", "job_id", pendingJob.ID, "error", err)
		}

		if index < len(pendingJobs)-1 {
			if err := p.sleepWithJitter(ctx); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Processor) processJob(ctx context.Context, pendingJob PendingJob) error {
	logger := p.logger.With("job_id", pendingJob.ID)
	logger.Info("processing job started", "search_term", pendingJob.SearchTerm)

	details, err := p.fetchJobDetails(ctx, pendingJob.ID)
	if err != nil {
		wrapped := fmt.Errorf("fetch linkedin job details: %w", err)
		if markErr := p.store.MarkJobErro(ctx, pendingJob.ID, wrapped.Error()); markErr != nil {
			return fmt.Errorf("%w; mark erro: %v", wrapped, markErr)
		}
		return wrapped
	}

	review, err := p.llm.ReviewJob(ctx, details)
	if err != nil {
		wrapped := fmt.Errorf("review job with llm: %w", err)
		if markErr := p.store.MarkJobErro(ctx, pendingJob.ID, wrapped.Error()); markErr != nil {
			return fmt.Errorf("%w; mark erro: %v", wrapped, markErr)
		}
		return wrapped
	}

	review.JobTitle = fallbackString(review.JobTitle, details.Title)
	review.Company = fallbackString(review.Company, details.Company)

	switch review.Decision {
	case "REJEITADO":
		if err := p.store.MarkJobProcessed(ctx, pendingJob.ID, review, details.ApplicationLink); err != nil {
			return err
		}
	case "APROVADO":
		if err := p.notifier.SendApprovedJob(ctx, pendingJob.ID, review, details.ApplicationLink); err != nil {
			wrapped := fmt.Errorf("send discord webhook: %w", err)
			if markErr := p.store.MarkJobErro(ctx, pendingJob.ID, wrapped.Error()); markErr != nil {
				return fmt.Errorf("%w; mark erro: %v", wrapped, markErr)
			}
			return wrapped
		}

		if err := p.store.MarkJobProcessed(ctx, pendingJob.ID, review, details.ApplicationLink); err != nil {
			return err
		}
	default:
		wrapped := fmt.Errorf("unexpected review decision %q", review.Decision)
		if markErr := p.store.MarkJobErro(ctx, pendingJob.ID, wrapped.Error()); markErr != nil {
			return fmt.Errorf("%w; mark erro: %v", wrapped, markErr)
		}
		return wrapped
	}

	logger.Info("processing job finished", "decision", review.Decision)
	return nil
}

func (p *Processor) fetchJobDetails(ctx context.Context, jobID string) (JobDetails, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(linkedInJobPostingURL, jobID), nil)
	if err != nil {
		return JobDetails{}, err
	}
	request.Header.Set("User-Agent", browserUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	response, err := p.client.Do(request)
	if err != nil {
		return JobDetails{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return JobDetails{}, fmt.Errorf("linkedin details request returned status %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return JobDetails{}, fmt.Errorf("read linkedin details response: %w", err)
	}

	jobDetails := parseJobDetails(jobID, string(body))
	if jobDetails.DescriptionText == "" {
		return JobDetails{}, errors.New("empty job description extracted from linkedin response")
	}

	return jobDetails, nil
}

func (p *Processor) sleepWithJitter(ctx context.Context) error {
	// Jitter between requests helps avoid using a fixed access pattern against LinkedIn.
	delay := time.Duration(3+p.rand.Intn(4)) * time.Second
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func parseJobDetails(jobID string, body string) JobDetails {
	return JobDetails{
		ID:              jobID,
		Title:           firstHTMLCapture(body, jobTitlePattern),
		Company:         firstHTMLCapture(body, companyNamePattern),
		DescriptionText: firstHTMLCapture(body, jobDescriptionPattern),
		ApplicationLink: fmt.Sprintf("https://www.linkedin.com/jobs/view/%s", jobID),
	}
}

func firstHTMLCapture(body string, pattern *regexp.Regexp) string {
	matches := pattern.FindStringSubmatch(body)
	for index := 1; index < len(matches); index++ {
		candidate := normalizeHTMLText(matches[index])
		if candidate != "" {
			return candidate
		}
	}

	return ""
}

func normalizeHTMLText(raw string) string {
	withoutTags := htmlTagPattern.ReplaceAllString(raw, " ")
	unescaped := html.UnescapeString(withoutTags)
	return strings.Join(strings.Fields(unescaped), " ")
}

func fallbackString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}

	return strings.TrimSpace(fallback)
}