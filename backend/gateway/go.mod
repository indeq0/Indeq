module github.com/cc-0000/indeq/gateway

go 1.23.5

replace github.com/cc-0000/indeq/authentication => ../authentication

require (
	github.com/cc-0000/indeq/common v0.0.0
	github.com/rabbitmq/amqp091-go v1.10.0
	google.golang.org/grpc v1.71.0
)

require (
	github.com/joho/godotenv v1.5.1 // indirect
	golang.org/x/net v0.37.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250313205543-e70fdf4c4cb4 // indirect
	google.golang.org/protobuf v1.36.6
)

replace (
	github.com/cc-0000/indeq/common => ../common
	github.com/cc-0000/indeq/query => ../query
)
