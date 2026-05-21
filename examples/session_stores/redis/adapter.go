// Package main provides a Redis-backed [claude.SessionStore] reference
// implementation.
//
// Storage layout:
//   - Transcript entries: Redis list at "ss:<project_key>/<session_id>[/<subpath>]"
//     (RPUSH for append, LRANGE 0 -1 for load — preserves insertion order).
//   - Session index: sorted set at "ss_idx:<project_key>" with UnixMilli mtime
//     as score and session_id as member (O(log N) updates, O(N) listing).
//
// Copy adapter.go into your project. Dependency: github.com/redis/go-redis/v9.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	claude "github.com/Flohs/claude-agent-sdk-go"
	"github.com/redis/go-redis/v9"
)

const ssPrefix = "ss"

// RedisSessionStore is a [claude.SessionStore] backed by Redis.
// It also satisfies [claude.SessionStoreLister], [claude.SessionStoreDeleter],
// and [claude.SessionStoreSubkeys].
type RedisSessionStore struct {
	c *redis.Client
}

// NewRedisSessionStore returns a store backed by the given Redis client.
func NewRedisSessionStore(c *redis.Client) *RedisSessionStore {
	return &RedisSessionStore{c: c}
}

func (s *RedisSessionStore) listKey(k claude.SessionKey) string {
	if k.Subpath == "" {
		return fmt.Sprintf("%s:%s/%s", ssPrefix, k.ProjectKey, k.SessionID)
	}
	return fmt.Sprintf("%s:%s/%s/%s", ssPrefix, k.ProjectKey, k.SessionID, k.Subpath)
}

func (s *RedisSessionStore) idxKey(pk string) string {
	return fmt.Sprintf("%s_idx:%s", ssPrefix, pk)
}

// Append satisfies [claude.SessionStore].
func (s *RedisSessionStore) Append(ctx context.Context, key claude.SessionKey, entries []claude.SessionStoreEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	pipe := s.c.Pipeline()
	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("redis store: marshal: %w", err)
		}
		pipe.RPush(ctx, s.listKey(key), string(data))
	}
	if key.Subpath == "" {
		pipe.ZAdd(ctx, s.idxKey(key.ProjectKey), redis.Z{
			Score:  float64(time.Now().UnixMilli()),
			Member: key.SessionID,
		})
	}
	_, err := pipe.Exec(ctx)
	return err
}

// Load satisfies [claude.SessionStore].
func (s *RedisSessionStore) Load(ctx context.Context, key claude.SessionKey) ([]claude.SessionStoreEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := s.c.LRange(ctx, s.listKey(key), 0, -1).Result()
	if err == redis.Nil || len(raw) == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis store: LRANGE: %w", err)
	}
	out := make([]claude.SessionStoreEntry, 0, len(raw))
	for _, r := range raw {
		var e claude.SessionStoreEntry
		if err := json.Unmarshal([]byte(r), &e); err != nil {
			return nil, fmt.Errorf("redis store: unmarshal: %w", err)
		}
		out = append(out, e)
	}
	return out, nil
}

// ListSessions satisfies [claude.SessionStoreLister].
func (s *RedisSessionStore) ListSessions(ctx context.Context, projectKey string) ([]claude.SessionStoreListEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	members, err := s.c.ZRangeWithScores(ctx, s.idxKey(projectKey), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("redis store: ZRANGEWITHSCORES: %w", err)
	}
	out := make([]claude.SessionStoreListEntry, len(members))
	for i, m := range members {
		out[i] = claude.SessionStoreListEntry{
			SessionID: fmt.Sprint(m.Member),
			Mtime:     int64(m.Score),
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mtime > out[j].Mtime })
	return out, nil
}

// Delete satisfies [claude.SessionStoreDeleter]. Deleting a main transcript
// (Subpath == "") cascades to all sibling subkeys via SCAN+DEL.
func (s *RedisSessionStore) Delete(ctx context.Context, key claude.SessionKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	pipe := s.c.Pipeline()
	pipe.Del(ctx, s.listKey(key))
	if key.Subpath == "" {
		pipe.ZRem(ctx, s.idxKey(key.ProjectKey), key.SessionID)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return fmt.Errorf("redis store: delete: %w", err)
	}
	if key.Subpath == "" {
		pattern := fmt.Sprintf("%s:%s/%s/*", ssPrefix, key.ProjectKey, key.SessionID)
		return s.deletePattern(ctx, pattern)
	}
	return nil
}

func (s *RedisSessionStore) deletePattern(ctx context.Context, pattern string) error {
	var cur uint64
	for {
		keys, next, err := s.c.Scan(ctx, cur, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("redis store: SCAN: %w", err)
		}
		if len(keys) > 0 {
			if err := s.c.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("redis store: DEL: %w", err)
			}
		}
		cur = next
		if cur == 0 {
			break
		}
	}
	return nil
}

// ListSubkeys satisfies [claude.SessionStoreSubkeys].
func (s *RedisSessionStore) ListSubkeys(ctx context.Context, key claude.SessionListSubkeysKey) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pattern := fmt.Sprintf("%s:%s/%s/*", ssPrefix, key.ProjectKey, key.SessionID)
	strip := fmt.Sprintf("%s:%s/%s/", ssPrefix, key.ProjectKey, key.SessionID)
	var out []string
	var cur uint64
	for {
		keys, next, err := s.c.Scan(ctx, cur, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("redis store: SCAN: %w", err)
		}
		for _, k := range keys {
			out = append(out, strings.TrimPrefix(k, strip))
		}
		cur = next
		if cur == 0 {
			break
		}
	}
	return out, nil
}
