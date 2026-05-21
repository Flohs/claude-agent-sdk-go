module github.com/Flohs/claude-agent-sdk-go/examples/session_stores/s3

go 1.26.1

require (
	github.com/Flohs/claude-agent-sdk-go v0.0.0-00010101000000-000000000000
	github.com/aws/aws-sdk-go-v2 v1.36.5
	github.com/aws/aws-sdk-go-v2/config v1.29.14
	github.com/aws/aws-sdk-go-v2/service/s3 v1.79.5
)

replace github.com/Flohs/claude-agent-sdk-go => ../../../
