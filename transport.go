package claude

import "context"

// Transport is the interface for low-level I/O with the Claude process or service.
//
// This is an internal API exposed for custom transport implementations.
// The Query layer builds on top of this to implement the control protocol.
type Transport interface {
	// Connect establishes the transport connection.
	Connect(ctx context.Context) error
	// Write sends raw data to the transport (typically JSON + newline).
	Write(data string) error
	// ReadMessages returns a channel that receives parsed JSON messages.
	ReadMessages(ctx context.Context) <-chan map[string]any
	// Close closes the transport and cleans up resources.
	Close() error
	// IsReady returns whether the transport is ready for communication.
	IsReady() bool
	// EndInput closes the input stream (stdin for subprocess transports).
	EndInput() error
}
