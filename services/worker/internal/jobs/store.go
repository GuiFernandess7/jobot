package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrMissingDatabaseURL = errors.New("DATABASE_URL is not set")

const (
	JobStatusPending   = "PENDENTE"
	JobStatusProcessed = "PROCESSADO"
	JobStatusErro      = "ERRO"
)

type PendingJob struct {
	ID         string
	SearchTerm string
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(ctx context.Context) (*Store, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return nil, ErrMissingDatabaseURL
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	config.MaxConns = 4
	config.MinConns = 1
	config.MaxConnIdleTime = 5 * time.Minute
	config.MaxConnLifetime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	store := &Store{pool: pool}
	if err := store.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}

	s.pool.Close()
}

func (s *Store) ensureSchema(ctx context.Context) error {
	const createTableQuery = `
CREATE TABLE IF NOT EXISTS vagas (
	id_vaga TEXT PRIMARY KEY,
	termo_busca TEXT NOT NULL,
	status_processamento TEXT NOT NULL DEFAULT 'PENDENTE',
	data_captura TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`

	const alterTableQuery = `
ALTER TABLE vagas
	ADD COLUMN IF NOT EXISTS justificativa TEXT,
	ADD COLUMN IF NOT EXISTS titulo_vaga TEXT,
	ADD COLUMN IF NOT EXISTS empresa TEXT,
	ADD COLUMN IF NOT EXISTS link_aplicacao TEXT,
	ADD COLUMN IF NOT EXISTS data_processamento TIMESTAMPTZ;`

	if _, err := s.pool.Exec(ctx, createTableQuery); err != nil {
		return fmt.Errorf("ensure vagas table: %w", err)
	}

	if _, err := s.pool.Exec(ctx, alterTableQuery); err != nil {
		return fmt.Errorf("ensure vagas columns: %w", err)
	}

	return nil
}

func (s *Store) FetchPendingJobs(ctx context.Context) ([]PendingJob, error) {
	const pendingJobsQuery = `
SELECT id_vaga, termo_busca
FROM vagas
WHERE status_processamento = $1
ORDER BY data_captura ASC;`

	rows, err := s.pool.Query(ctx, pendingJobsQuery, JobStatusPending)
	if err != nil {
		return nil, fmt.Errorf("query pending jobs: %w", err)
	}
	defer rows.Close()

	pendingJobs := make([]PendingJob, 0)
	for rows.Next() {
		var job PendingJob
		if err := rows.Scan(&job.ID, &job.SearchTerm); err != nil {
			return nil, fmt.Errorf("scan pending job: %w", err)
		}
		pendingJobs = append(pendingJobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending jobs: %w", err)
	}

	return pendingJobs, nil
}

func (s *Store) MarkJobProcessed(ctx context.Context, jobID string, review JobReview, applicationLink string) error {
	const processedQuery = `
UPDATE vagas
SET status_processamento = $2,
	justificativa = $3,
	titulo_vaga = $4,
	empresa = $5,
	link_aplicacao = $6,
	data_processamento = NOW()
WHERE id_vaga = $1;`

	if _, err := s.pool.Exec(
		ctx,
		processedQuery,
		jobID,
		JobStatusProcessed,
		review.Justification,
		review.JobTitle,
		review.Company,
		applicationLink,
	); err != nil {
		return fmt.Errorf("mark job %s as processed: %w", jobID, err)
	}

	return nil
}

func (s *Store) MarkJobErro(ctx context.Context, jobID string, justification string) error {
	const erroQuery = `
UPDATE vagas
SET status_processamento = $2,
	justificativa = $3,
	data_processamento = NOW()
WHERE id_vaga = $1;`

	if _, err := s.pool.Exec(ctx, erroQuery, jobID, JobStatusErro, justification); err != nil {
		return fmt.Errorf("mark job %s as erro: %w", jobID, err)
	}

	return nil
}