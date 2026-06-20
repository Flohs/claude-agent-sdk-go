package claude

// Model constants for use with [Options.Model] and [Options.FallbackModel].
// Using a constant avoids typos and provides autocomplete.
//
// Short alias constants (e.g. [ModelFable], [ModelOpus], [ModelSonnet],
// [ModelHaiku]) map to the same strings the Claude Code CLI accepts as
// model shorthands.
const (
	// Claude Fable 5 — the most recent Fable-family model.
	// Port of TypeScript SDK v0.3.170.
	ModelFable5 = "claude-fable-5"
	// ModelFable is the short CLI alias for the Fable model family.
	// Port of TypeScript SDK v0.3.170.
	ModelFable = "fable"

	// Claude Opus 4.8 — highest capability model in the Claude 4 family.
	ModelOpus48 = "claude-opus-4-8"
	// ModelOpus is the short CLI alias for the Opus model family.
	ModelOpus = "opus"

	// Claude Sonnet 4.6 — balanced capability and speed in the Claude 4 family.
	ModelSonnet46 = "claude-sonnet-4-6"
	// ModelSonnet is the short CLI alias for the Sonnet model family.
	ModelSonnet = "sonnet"

	// Claude Haiku 4.5 — fastest and most compact model in the Claude 4 family.
	ModelHaiku45 = "claude-haiku-4-5-20251001"
	// ModelHaiku is the short CLI alias for the Haiku model family.
	ModelHaiku = "haiku"
)
