package jobs

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const linkedInGuestSearchURL = "https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search"

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"

var jobPostingCardKeyPattern = regexp.MustCompile(`jobPostingCardKey":"(\d+)"`)
var jobViewURLPattern = regexp.MustCompile(`/view/[^\"?]*?(\d{6,})`)

type CapturedJob struct {
	ID         string
	SearchTerm string
}

type LinkedInScraper struct {
	httpClient *http.Client
	rand       *rand.Rand
}

func NewLinkedInScraper() *LinkedInScraper {
	return &LinkedInScraper{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *LinkedInScraper) GetJobIDsFromLinkedIn(ctx context.Context, terms []string) ([]CapturedJob, error) {
	jobs := make([]CapturedJob, 0)
	seen := make(map[string]struct{})

	for index, term := range terms {
		termJobs, err := s.fetchTermJobs(ctx, term)
		if err != nil {
			return nil, err
		}

		for _, job := range termJobs {
			key := job.ID + "|" + job.SearchTerm
			if _, exists := seen[key]; exists {
				continue
			}

			seen[key] = struct{}{}
			jobs = append(jobs, job)
		}

		if index < len(terms)-1 {
			// Jitter reduces the chance of repeating the same timing signature between requests.
			delay := time.Duration(4+s.rand.Intn(6)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return jobs, nil
}

func (s *LinkedInScraper) fetchTermJobs(ctx context.Context, term string) ([]CapturedJob, error) {
	requestURL, err := buildLinkedInGuestSearchURL(term)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build linkedin request: %w", err)
	}
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request linkedin jobs for %q: %w", term, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linkedin jobs request for %q returned status %d", term, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read linkedin response body for %q: %w", term, err)
	}

	return parseJobIDs(string(body), term), nil
}

func buildLinkedInGuestSearchURL(term string) (string, error) {
	parsedURL, err := url.Parse(linkedInGuestSearchURL)
	if err != nil {
		return "", fmt.Errorf("parse linkedin guest api url: %w", err)
	}

	query := parsedURL.Query()
	query.Set("keywords", term)
	query.Set("location", "Brazil")
	query.Set("geoId", "106057199")
	query.Set("f_TPR", "r86400")
	query.Set("start", "0")
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String(), nil
}

func parseJobIDs(body string, term string) []CapturedJob {
	matches := make([]CapturedJob, 0)
	seenIDs := make(map[string]struct{})

	collectMatch := func(match string) {
		jobID := strings.TrimSpace(match)
		if jobID == "" {
			return
		}
		if _, exists := seenIDs[jobID]; exists {
			return
		}

		seenIDs[jobID] = struct{}{}
		matches = append(matches, CapturedJob{ID: jobID, SearchTerm: term})
	}

	for _, groups := range jobPostingCardKeyPattern.FindAllStringSubmatch(body, -1) {
		if len(groups) > 1 {
			collectMatch(groups[1])
		}
	}

	for _, groups := range jobViewURLPattern.FindAllStringSubmatch(body, -1) {
		if len(groups) > 1 {
			collectMatch(groups[1])
		}
	}

	return matches
}