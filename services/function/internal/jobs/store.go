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

func (s *Store) SaveJobsToDatabase(ctx context.Context, jobs []CapturedJob) (int64, error) {
	if len(jobs) == 0 {
		return 0, nil
	}

	const insertJobQuery = `
INSERT INTO vagas (id_vaga, termo_busca, status_processamento, data_captura)
VALUES ($1, $2, 'PENDENTE', NOW())
ON CONFLICT (id_vaga) DO NOTHING;`

	var inserted int64
	for _, job := range jobs {
		commandTag, err := s.pool.Exec(ctx, insertJobQuery, job.ID, job.SearchTerm)
		if err != nil {
			return inserted, fmt.Errorf("insert job %s: %w", job.ID, err)
		}
		inserted += commandTag.RowsAffected()
	}

	return inserted, nil
}