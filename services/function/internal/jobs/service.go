package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

var ErrNoSearchTerms = errors.New("at least one search term must be provided")

var defaultSearchTerms = []string{"golang", "python backend"}

type Service struct {
	logger  *slog.Logger
	scraper *LinkedInScraper
	store   *Store
}

func NewService(logger *slog.Logger) (*Service, error) {
	store, err := NewStore(context.Background())
	if err != nil {
		return nil, fmt.Errorf("create jobs store: %w", err)
	}

	return &Service{
		logger:  logger,
		scraper: NewLinkedInScraper(),
		store:   store,
	}, nil
}

func (s *Service) NormalizeTerms(terms []string) []string {
	if len(terms) == 0 {
		return append([]string(nil), defaultSearchTerms...)
	}

	normalized := make([]string, 0, len(terms))
	for _, term := range terms {
		trimmed := strings.TrimSpace(term)
		if trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}

	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func (s *Service) StartCapture(requestID string, terms []string) error {
	normalized := s.NormalizeTerms(terms)
	if len(normalized) == 0 {
		return ErrNoSearchTerms
	}

	go s.runCapture(requestID, normalized)
	return nil
}

func (s *Service) runCapture(requestID string, terms []string) {
	logger := s.logger.With("request_id", requestID)
	logger.Info("job capture started", "terms", terms)

	jobs, err := s.scraper.GetJobIDsFromLinkedIn(context.Background(), terms)
	if err != nil {
		logger.Error("job capture failed", "error", err)
		return
	}

	inserted, err := s.store.SaveJobsToDatabase(context.Background(), jobs)
	if err != nil {
		logger.Error("job persistence failed", "error", err)
		return
	}

	logger.Info("job capture finished", "captured_jobs", len(jobs), "inserted_jobs", inserted)
}