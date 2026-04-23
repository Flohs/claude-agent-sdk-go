package claude

import (
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
