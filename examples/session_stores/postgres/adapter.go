// Package main provides a PostgreSQL-backed [claude.SessionStore] reference
// implementation.
//
// Schema (run InitSchema once before first use):
//
//	CREATE TABLE IF NOT EXISTS claude_sessions (
//	    id          BIGSERIAL PRIMARY KEY,
//	    project_key TEXT    NOT NULL,
//	    session_id  TEXT    NOT NULL,
//	    subpath     TEXT    NOT NULL DEFAULT '',
//	    data        JSONB   NOT NULL,
//	    mtime       BIGINT  NOT NULL
//	);
//	CREATE INDEX IF NOT EXISTS claude_sessions_lookup
//	    ON claude_sessions (project_key, session_id, subpath, id);
//
// Copy adapter.go into your project. Dependency: github.com/jackc/pgx/v5.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	claude "github.com/Flohs/claude-agent-sdk-go"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSessionStore is a [claude.SessionStore] backed by PostgreSQL.
// It also satisfies [claude.SessionStoreLister], [claude.SessionStoreDeleter],
// and [claude.SessionStoreSubkeys].
type PostgresSessionStore struct {
	pool *pgxpool.Pool
}

// NewPostgresSessionStore returns a store backed by the given connection pool.
func NewPostgresSessionStore(pool *pgxpool.Pool) *PostgresSessionStore {
	return &PostgresSessionStore{pool: pool}
}

// InitSchema creates the sessions table and index if they do not exist.
// Call once during application startup.
func (s *PostgresSessionStore) InitSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS claude_sessions (
		    id          BIGSERIAL PRIMARY KEY,
		    project_key TEXT    NOT NULL,
		    session_id  TEXT    NOT NULL,
		    subpath     TEXT    NOT NULL DEFAULT '',
		    data        JSONB   NOT NULL,
		    mtime       BIGINT  NOT NULL
		);
		CREATE INDEX IF NOT EXISTS claude_sessions_lookup
		    ON claude_sessions (project_key, session_id, subpath, id);
	`)
	return err
}

// Append satisfies [claude.SessionStore].
func (s *PostgresSessionStore) Append(ctx context.Context, key claude.SessionKey, entries []claude.SessionStoreEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	mtime := time.Now().UnixMilli()
	batch := &pgx.Batch{}
	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("postgres store: marshal: %w", err)
		}
		batch.Queue(
			`INSERT INTO claude_sessions (project_key, session_id, subpath, data, mtime) VALUES ($1, $2, $3, $4, $5)`,
			key.ProjectKey, key.SessionID, key.Subpath, data, mtime,
		)
	}
	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range entries {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("postgres store: insert: %w", err)
		}
	}
	return results.Close()
}

// Load satisfies [claude.SessionStore].
func (s *PostgresSessionStore) Load(ctx context.Context, key claude.SessionKey) ([]claude.SessionStoreEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT data FROM claude_sessions WHERE project_key=$1 AND session_id=$2 AND subpath=$3 ORDER BY id`,
		key.ProjectKey, key.SessionID, key.Subpath,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres store: query: %w", err)
	}
	defer rows.Close()
	var out []claude.SessionStoreEntry
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("postgres store: scan: %w", err)
		}
		var e claude.SessionStoreEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, fmt.Errorf("postgres store: unmarshal: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListSessions satisfies [claude.SessionStoreLister]. Returns only main
// transcript sessions (subpath == '') ordered by most-recently-written mtime.
func (s *PostgresSessionStore) ListSessions(ctx context.Context, projectKey string) ([]claude.SessionStoreListEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT session_id, MAX(mtime) AS mtime
		   FROM claude_sessions
		  WHERE project_key=$1 AND subpath=''
		  GROUP BY session_id
		  ORDER BY mtime DESC`,
		projectKey,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres store: list sessions: %w", err)
	}
	defer rows.Close()
	var out []claude.SessionStoreListEntry
	for rows.Next() {
		var e claude.SessionStoreListEntry
		if err := rows.Scan(&e.SessionID, &e.Mtime); err != nil {
			return nil, fmt.Errorf("postgres store: scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Delete satisfies [claude.SessionStoreDeleter]. Deleting a main transcript
// (Subpath == "") cascades to all sibling subkeys via a single DELETE.
func (s *PostgresSessionStore) Delete(ctx context.Context, key claude.SessionKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key.Subpath == "" {
		// Delete the main transcript and all subkeys in one query.
		_, err := s.pool.Exec(ctx,
			`DELETE FROM claude_sessions WHERE project_key=$1 AND session_id=$2`,
			key.ProjectKey, key.SessionID,
		)
		return err
	}
	_, err := s.pool.Exec(ctx,
		`DELETE FROM claude_sessions WHERE project_key=$1 AND session_id=$2 AND subpath=$3`,
		key.ProjectKey, key.SessionID, key.Subpath,
	)
	return err
}

// ListSubkeys satisfies [claude.SessionStoreSubkeys].
func (s *PostgresSessionStore) ListSubkeys(ctx context.Context, key claude.SessionListSubkeysKey) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT subpath FROM claude_sessions
		  WHERE project_key=$1 AND session_id=$2 AND subpath!=''`,
		key.ProjectKey, key.SessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres store: list subkeys: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var sp string
		if err := rows.Scan(&sp); err != nil {
			return nil, fmt.Errorf("postgres store: scan: %w", err)
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}
