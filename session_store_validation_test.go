package claude

import (
	"context"
	"errors"
	"testing"
)

func TestValidateSessionStoreOptions_NilOptions(t *testing.T) {
	if err := validateSessionStoreOptions(nil); err != nil {
		t.Fatalf("expected nil for nil options, got %v", err)
	}
}

func TestValidateSessionStoreOptions_NoStore(t *testing.T) {
	if err := validateSessionStoreOptions(&Options{}); err != nil {
		t.Fatalf("expected nil when SessionStore unset, got %v", err)
	}
}

func TestValidateSessionStoreOptions_StoreAlone(t *testing.T) {
	opts := &Options{SessionStore: NewInMemorySessionStore()}
	if err := validateSessionStoreOptions(opts); err != nil {
		t.Fatalf("expected nil for SessionStore alone, got %v", err)
	}
}

func TestValidateSessionStoreOptions_RejectsCheckpointing(t *testing.T) {
	opts := &Options{
		SessionStore:            NewInMemorySessionStore(),
		EnableFileCheckpointing: true,
	}
	err := validateSessionStoreOptions(opts)
	if err == nil {
		t.Fatal("expected error when SessionStore combined with EnableFileCheckpointing")
	}
	if !errors.Is(err, ErrSessionStoreWithFileCheckpointing) {
		t.Fatalf("expected ErrSessionStoreWithFileCheckpointing, got %v", err)
	}
}

func TestValidateSessionStoreOptions_CheckpointingAloneOK(t *testing.T) {
	opts := &Options{EnableFileCheckpointing: true}
	if err := validateSessionStoreOptions(opts); err != nil {
		t.Fatalf("expected nil when only EnableFileCheckpointing is set, got %v", err)
	}
}

// mockNoListerStore is a minimal SessionStore that does NOT implement
// SessionStoreLister; used to exercise the ContinueConversation validation rule.
type mockNoListerStore struct{}

func (mockNoListerStore) Append(ctx context.Context, key SessionKey, entries []SessionStoreEntry) error {
	return nil
}
func (mockNoListerStore) Load(ctx context.Context, key SessionKey) ([]SessionStoreEntry, error) {
	return nil, nil
}

func TestValidateSessionStoreOptions_ContinueWithoutListerRejected(t *testing.T) {
	opts := &Options{
		SessionStore:         mockNoListerStore{},
		ContinueConversation: true,
	}
	err := validateSessionStoreOptions(opts)
	if err == nil {
		t.Fatal("expected error when ContinueConversation combined with SessionStore lacking SessionStoreLister")
	}
	if !errors.Is(err, ErrSessionStoreContinueRequiresLister) {
		t.Fatalf("expected ErrSessionStoreContinueRequiresLister, got %v", err)
	}
}

func TestValidateSessionStoreOptions_ContinueWithResumeBypassesListerCheck(t *testing.T) {
	// Resume wins over Continue → ListSessions() is never called, so a
	// minimal store is fine even with ContinueConversation flipped on.
	opts := &Options{
		SessionStore:         mockNoListerStore{},
		ContinueConversation: true,
		Resume:               "11111111-2222-3333-4444-555555555555",
	}
	if err := validateSessionStoreOptions(opts); err != nil {
		t.Fatalf("expected nil when Resume is set, got %v", err)
	}
}

func TestValidateSessionStoreOptions_ContinueWithListerOK(t *testing.T) {
	opts := &Options{
		SessionStore:         NewInMemorySessionStore(), // implements SessionStoreLister
		ContinueConversation: true,
	}
	if err := validateSessionStoreOptions(opts); err != nil {
		t.Fatalf("expected nil when store implements SessionStoreLister, got %v", err)
	}
}
