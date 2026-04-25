package scan

import (
	"context"
	"fmt"
	"time"

	"github.com/sydlexius/mxlrcsvc-go/internal/models"
	"github.com/sydlexius/mxlrcsvc-go/internal/scanner"
)

// LibraryLister lists configured library roots.
type LibraryLister interface {
	List(ctx context.Context) ([]models.Library, error)
}

// ResultStore persists scan results.
type ResultStore interface {
	Upsert(ctx context.Context, libraryID int64, results []models.ScanResult) error
}

// LibraryScanner scans a library path.
type LibraryScanner interface {
	ScanLibrary(root string, opts scanner.ScanOptions) ([]models.ScanResult, error)
}

// OnCompleteFunc is called after a library scan has been persisted.
type OnCompleteFunc func(ctx context.Context, lib models.Library, results []models.ScanResult) error

// Scheduler periodically scans configured libraries and persists results.
type Scheduler struct {
	Libraries      LibraryLister
	Results        ResultStore
	Scanner        LibraryScanner
	Options        scanner.ScanOptions
	Interval       time.Duration
	MaxRuntime     time.Duration
	OnScanComplete OnCompleteFunc
}

// RunOnce scans every configured library exactly once.
func (s *Scheduler) RunOnce(ctx context.Context) error {
	if s.Libraries == nil {
		return fmt.Errorf("scan: scheduler libraries dependency is nil")
	}
	if s.Results == nil {
		return fmt.Errorf("scan: scheduler results dependency is nil")
	}
	if s.Scanner == nil {
		return fmt.Errorf("scan: scheduler scanner dependency is nil")
	}

	libs, err := s.Libraries.List(ctx)
	if err != nil {
		return fmt.Errorf("scan: list libraries: %w", err)
	}
	for _, lib := range libs {
		results, err := s.Scanner.ScanLibrary(lib.Path, s.Options)
		if err != nil {
			return fmt.Errorf("scan: scan library %d: %w", lib.ID, err)
		}
		for i := range results {
			results[i].LibraryID = lib.ID
			if results[i].Status == "" {
				results[i].Status = StatusPending
			}
		}
		if err := s.Results.Upsert(ctx, lib.ID, results); err != nil {
			return fmt.Errorf("scan: persist library %d: %w", lib.ID, err)
		}
		if s.OnScanComplete != nil {
			if err := s.OnScanComplete(ctx, lib, results); err != nil {
				return fmt.Errorf("scan: complete library %d: %w", lib.ID, err)
			}
		}
	}
	return nil
}

// Run scans immediately and then repeats at Interval until ctx is canceled or
// MaxRuntime elapses. If Interval is zero or negative, it performs one scan.
func (s *Scheduler) Run(ctx context.Context) error {
	if s.MaxRuntime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.MaxRuntime)
		defer cancel()
	}

	if err := s.RunOnce(ctx); err != nil {
		return err
	}
	if s.Interval <= 0 {
		return nil
	}

	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.RunOnce(ctx); err != nil {
				return err
			}
		}
	}
}
