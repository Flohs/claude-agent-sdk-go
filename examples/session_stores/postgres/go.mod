module github.com/Flohs/claude-agent-sdk-go/examples/session_stores/postgres

go 1.26.1

require (
	github.com/Flohs/claude-agent-sdk-go/v3 v3.0.0-00010101000000-000000000000
	github.com/jackc/pgx/v5 v5.9.2
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/Flohs/claude-agent-sdk-go/v3 => ../../../
