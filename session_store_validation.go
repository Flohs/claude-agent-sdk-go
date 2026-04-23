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
//
// Future sub-issues will add a ContinueConversation + Lister check.
func validateSessionStoreOptions(opts *Options) error {
	if opts == nil || opts.SessionStore == nil {
		return nil
	}
	if opts.EnableFileCheckpointing {
		return ErrSessionStoreWithFileCheckpointing
	}
	return nil
}
