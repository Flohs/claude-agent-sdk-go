// Package main provides an Amazon S3-backed [claude.SessionStore] reference
// implementation.
//
// Storage layout: one S3 object per [claude.SessionStore.Append] call, keyed
// as "<project_key>/<session_id>[/<subpath>]/part-<unix_nano>.jsonl".
// [Load] issues a ListObjectsV2 to enumerate parts in insertion order, then
// downloads each part via GetObject. This makes Append cheap (O(1) PUT)
// at the cost of a chatty Load (N GETs for N parts). For production use,
// coalescing parts on Load or after a session ends is recommended.
//
// Copy adapter.go into your project. Dependencies:
//
//	github.com/aws/aws-sdk-go-v2/config
//	github.com/aws/aws-sdk-go-v2/service/s3
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	claude "github.com/Flohs/claude-agent-sdk-go"
)

// S3SessionStore is a [claude.SessionStore] backed by Amazon S3.
// It also satisfies [claude.SessionStoreLister], [claude.SessionStoreDeleter],
// and [claude.SessionStoreSubkeys].
type S3SessionStore struct {
	client *s3.Client
	bucket string
}

// NewS3SessionStore returns a store backed by the given S3 client and bucket.
func NewS3SessionStore(client *s3.Client, bucket string) *S3SessionStore {
	return &S3SessionStore{client: client, bucket: bucket}
}

// keyPrefix returns the S3 key prefix for a transcript stream.
func keyPrefix(key claude.SessionKey) string {
	if key.Subpath == "" {
		return fmt.Sprintf("%s/%s/", key.ProjectKey, key.SessionID)
	}
	return fmt.Sprintf("%s/%s/%s/", key.ProjectKey, key.SessionID, key.Subpath)
}

// partKey returns a unique, time-ordered S3 object key for a single Append call.
func partKey(key claude.SessionKey) string {
	return fmt.Sprintf("%spart-%d.jsonl", keyPrefix(key), time.Now().UnixNano())
}

// Append satisfies [claude.SessionStore]. Each call writes one S3 object
// containing the JSON-lines for the provided entries.
func (s *S3SessionStore) Append(ctx context.Context, key claude.SessionKey, entries []claude.SessionStoreEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("s3 store: encode entry: %w", err)
		}
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(partKey(key)),
		Body:        bytes.NewReader(buf.Bytes()),
		ContentType: aws.String("application/x-ndjson"),
	})
	if err != nil {
		return fmt.Errorf("s3 store: PutObject: %w", err)
	}
	return nil
}

// Load satisfies [claude.SessionStore]. Retrieves all parts in insertion
// order (lexicographic key order, which equals time order because part keys
// embed UnixNano) and concatenates their entries.
func (s *S3SessionStore) Load(ctx context.Context, key claude.SessionKey) ([]claude.SessionStoreEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pfx := keyPrefix(key)
	var out []claude.SessionStoreEntry
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(pfx),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 store: ListObjectsV2: %w", err)
		}
		for _, obj := range page.Contents {
			entries, err := s.downloadPart(ctx, aws.ToString(obj.Key))
			if err != nil {
				return nil, err
			}
			out = append(out, entries...)
		}
	}
	return out, nil
}

func (s *S3SessionStore) downloadPart(ctx context.Context, objKey string) ([]claude.SessionStoreEntry, error) {
	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objKey),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 store: GetObject %s: %w", objKey, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("s3 store: read body: %w", err)
	}
	var entries []claude.SessionStoreEntry
	dec := json.NewDecoder(bytes.NewReader(body))
	for dec.More() {
		var e claude.SessionStoreEntry
		if err := dec.Decode(&e); err != nil {
			return nil, fmt.Errorf("s3 store: decode entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// ListSessions satisfies [claude.SessionStoreLister]. Lists main-transcript
// sessions under the project prefix, ordered by most-recently-modified part.
func (s *S3SessionStore) ListSessions(ctx context.Context, projectKey string) ([]claude.SessionStoreListEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pfx := projectKey + "/"
	sessionMtime := map[string]int64{}
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(pfx),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 store: ListObjectsV2: %w", err)
		}
		for _, obj := range page.Contents {
			rel := strings.TrimPrefix(aws.ToString(obj.Key), pfx)
			parts := strings.SplitN(rel, "/", 3)
			if len(parts) < 2 {
				continue
			}
			sessionID := parts[0]
			// Only count direct session parts (not subkey parts).
			if len(parts) == 2 && strings.HasPrefix(parts[1], "part-") {
				mtime := obj.LastModified.UnixMilli()
				if mtime > sessionMtime[sessionID] {
					sessionMtime[sessionID] = mtime
				}
			}
		}
	}
	out := make([]claude.SessionStoreListEntry, 0, len(sessionMtime))
	for sid, mt := range sessionMtime {
		out = append(out, claude.SessionStoreListEntry{SessionID: sid, Mtime: mt})
	}
	// Sort by mtime descending.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Mtime > out[j-1].Mtime; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// Delete satisfies [claude.SessionStoreDeleter]. Deletes all objects with
// the key's prefix. When Subpath is empty, this cascades to all subkeys.
func (s *S3SessionStore) Delete(ctx context.Context, key claude.SessionKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Collect all object keys under the prefix.
	pfx := keyPrefix(key)
	if key.Subpath == "" {
		// Also delete the main prefix without trailing component.
		pfx = fmt.Sprintf("%s/%s/", key.ProjectKey, key.SessionID)
	}
	var toDelete []types.ObjectIdentifier
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(pfx),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("s3 store: ListObjectsV2: %w", err)
		}
		for _, obj := range page.Contents {
			toDelete = append(toDelete, types.ObjectIdentifier{Key: obj.Key})
		}
	}
	if len(toDelete) == 0 {
		return nil
	}
	// S3 DeleteObjects accepts up to 1000 objects per call.
	for i := 0; i < len(toDelete); i += 1000 {
		end := i + 1000
		if end > len(toDelete) {
			end = len(toDelete)
		}
		_, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket),
			Delete: &types.Delete{Objects: toDelete[i:end], Quiet: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("s3 store: DeleteObjects: %w", err)
		}
	}
	return nil
}

// ListSubkeys satisfies [claude.SessionStoreSubkeys]. Enumerates unique
// subpath values stored under the given session.
func (s *S3SessionStore) ListSubkeys(ctx context.Context, key claude.SessionListSubkeysKey) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pfx := fmt.Sprintf("%s/%s/", key.ProjectKey, key.SessionID)
	seen := map[string]bool{}
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(pfx),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 store: ListObjectsV2: %w", err)
		}
		for _, obj := range page.Contents {
			rel := strings.TrimPrefix(aws.ToString(obj.Key), pfx)
			parts := strings.SplitN(rel, "/", 2)
			// Subkeys have at least two components: <subpath>/<part-file>.
			if len(parts) == 2 && parts[0] != "" {
				seen[parts[0]] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for sp := range seen {
		out = append(out, sp)
	}
	return out, nil
}
