package sessionquery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// Service encapsulates database indexing and search capabilities.
type Service struct {
	db      *DB
	indexer *Indexer
	mu      sync.RWMutex
}

var (
	defaultServiceInstance *Service
	defaultServiceOnce     sync.Once
	defaultServiceErr      error
)

// DefaultService returns the process-wide session query service.
func DefaultService() (*Service, error) {
	defaultServiceOnce.Do(func() {
		dbPath := storage.CacheDir() + "/session_query.db"
		svc, err := NewService(dbPath, storage.SessionsDir())
		if err != nil {
			defaultServiceErr = err
			return
		}
		defaultServiceInstance = svc
	})
	if defaultServiceErr != nil {
		return nil, defaultServiceErr
	}
	return defaultServiceInstance, nil
}

// NewService creates a new SessionQuery service with the specified paths.
func NewService(dbPath, sessionsDir string) (*Service, error) {
	db, err := OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sessionquery DB: %w", err)
	}
	indexer := NewIndexer(db, sessionsDir)
	return &Service{
		db:      db,
		indexer: indexer,
	}, nil
}

// Close closes the underlying resources.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Indexer returns the underlying session indexer.
func (s *Service) Indexer() *Indexer {
	return s.indexer
}

// DB returns the underlying DB instance.
func (s *Service) DB() *DB {
	return s.db
}

// Search runs a full-text search against the indexed sessions.
// Automatically ensures the index is synchronized first.
func (s *Service) Search(ctx context.Context, params SearchParams) (*SearchResponse, error) {
	// Sync index before searching with a short timeout
	syncCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	_, _ = s.indexer.SyncAll(syncCtx)
	cancel()

	return s.db.Search(ctx, params)
}

// RebuildIndex drops and rebuilds the full-text search index from scratch.
func (s *Service) RebuildIndex(ctx context.Context) error {
	return s.indexer.RebuildIndex(ctx)
}
