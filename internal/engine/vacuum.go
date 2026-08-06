package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// VacuumInfo reports what a VACUUM of a store would reclaim before it runs, so
// the caller can decide whether the rewrite is worth it.
type VacuumInfo struct {
	Store string
	// FreelistBytes is the space currently held by free pages (page_size times
	// freelist_count). VACUUM shrinks the file by approximately this amount.
	FreelistBytes int64
	// SizeBytes is the current file size on disk.
	SizeBytes int64
}

// VacuumSizes reports each store's current size and reclaimable freelist, so a
// sweep can tell the user how much a VACUUM would free before asking. Missing
// stores are omitted.
func VacuumSizes(ctx context.Context, stores []string) ([]VacuumInfo, error) {
	var out []VacuumInfo
	for _, path := range stores {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		}
		info, err := vacuumInfo(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("measure %s: %w", path, err)
		}
		out = append(out, info)
	}
	return out, nil
}

// Vacuum runs VACUUM plus a truncating WAL checkpoint on one store, returning
// how many bytes were freed (before minus after file size). It is idempotent:
// a store with no free pages is a no-op. The caller is responsible for
// ensuring no live agent holds the store; VACUUM needs an exclusive lock.
func Vacuum(ctx context.Context, store string) (int64, error) {
	if _, err := os.Stat(store); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	before, err := fileSize(store)
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", store, err)
	}
	db, err := openStore(store)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", store, err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
		return 0, fmt.Errorf("%s: vacuum: %w", store, err)
	}
	// WAL journals keep the main file from shrinking; a truncating checkpoint
	// folds the WAL back and, with an empty WAL, lets VACUUM's shrink stand.
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return 0, fmt.Errorf("%s: wal checkpoint: %w", store, err)
	}
	after, err := fileSize(store)
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", store, err)
	}
	if after > before {
		after = before
	}
	return before - after, nil
}

// vacuumInfo reads the freelist and file size of one store.
func vacuumInfo(ctx context.Context, path string) (VacuumInfo, error) {
	info := VacuumInfo{Store: path}
	fi, err := os.Stat(path)
	if err != nil {
		return info, err
	}
	info.SizeBytes = fi.Size()

	db, err := openReadOnlyStore(path)
	if err != nil {
		return info, err
	}
	defer db.Close()
	var pageSize, freelist int64
	if err := db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return info, err
	}
	if err := db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&freelist); err != nil {
		return info, err
	}
	info.FreelistBytes = pageSize * freelist
	return info, nil
}

func openReadOnlyStore(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func fileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}
