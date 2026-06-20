package claude

// Model string constants for use with [Options.Model].
//
// Short aliases ([ModelFable], [ModelOpus], [ModelSonnet], [ModelHaiku])
// resolve server-side to the latest generation of that family;
// use the versioned constants ([ModelFable5], [ModelOpus48], etc.) for
// pinned, reproducible deployments.
const (
	// ModelFable is the short alias that always resolves to the latest
	// Claude Fable generation. Port of TypeScript SDK v0.3.170.
	ModelFable = "fable"
	// ModelOpus is the short alias that always resolves to the latest
	// Claude Opus generation.
	ModelOpus = "opus"
	// ModelSonnet is the short alias that always resolves to the latest
	// Claude Sonnet generation.
	ModelSonnet = "sonnet"
	// ModelHaiku is the short alias that always resolves to the latest
	// Claude Haiku generation.
	ModelHaiku = "haiku"

	// ModelFable5 is the versioned identifier for Claude Fable 5.
	// Port of TypeScript SDK v0.3.170.
	ModelFable5 = "claude-fable-5"
	// ModelOpus48 is the versioned identifier for Claude Opus 4.8.
	ModelOpus48 = "claude-opus-4-8"
	// ModelSonnet46 is the versioned identifier for Claude Sonnet 4.6.
	ModelSonnet46 = "claude-sonnet-4-6"
	// ModelHaiku45 is the versioned identifier for Claude Haiku 4.5.
	ModelHaiku45 = "claude-haiku-4-5-20251001"
)
