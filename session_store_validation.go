package claude

import "errors"

// Errors returned by validateSessionStoreOptions. Exposed as sentinels so
// callers can branch on specific validation failures (e.g. via errors.Is).
var (
	// ErrSessionStoreWithFileCheckpointing is returned when both
	// Options.SessionStore and Options.EnableFileCheckpointing are set.
	// Checkpoints are local-disk only and would diverge from the mirrored
	// transcript, so the combination is rejected.
	ErrSessionStoreWithFileCheckpointing = errors.New(
		"session_store cannot be combined with EnableFileCheckpointing " +
			"(checkpoints are local-disk only and would diverge from the " +
			"mirrored transcript)",
	)

	// ErrSessionStoreContinueRequiresLister is returned when
	// Options.ContinueConversation is set alongside Options.SessionStore but
	// the store does not implement [SessionStoreLister]. Without
	// ListSessions() the SDK cannot resolve the newest session to resume,
	// so the combination is rejected pre-flight instead of surfacing as a
	// confusing mid-session error.
	//
	// An explicit Options.Resume bypasses this requirement (resume wins
	// over continue and never invokes ListSessions).
	ErrSessionStoreContinueRequiresLister = errors.New(
		"ContinueConversation with a SessionStore requires the store to " +
			"implement SessionStoreLister",
	)
)

// validateSessionStoreOptions rejects invalid [Options.SessionStore] option
// combinations before the subprocess is spawned so misconfiguration fails
// fast instead of surfacing as a confusing runtime error mid-session.
//
// Currently enforced:
//
//   - SessionStore + EnableFileCheckpointing → rejected. The CLI's file
//     checkpoint store is local-disk only and would diverge from the
//     mirrored transcript.
//   - SessionStore + ContinueConversation without Resume, where the store
//     does not implement [SessionStoreLister] → rejected. Resolving the
//     newest session requires ListSessions(); minimal stores cannot.
func validateSessionStoreOptions(opts *Options) error {
	if opts == nil || opts.SessionStore == nil {
		return nil
	}
	if opts.EnableFileCheckpointing {
		return ErrSessionStoreWithFileCheckpointing
	}
	if opts.ContinueConversation && opts.Resume == "" {
		if _, ok := opts.SessionStore.(SessionStoreLister); !ok {
			return ErrSessionStoreContinueRequiresLister
		}
	}
	return nil
}
