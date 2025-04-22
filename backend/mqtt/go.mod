module github.com/cc-0000/indeq/mqtt

go 1.23.5

replace github.com/cc-0000/indeq/common => ../common

require (
	github.com/cc-0000/indeq/common v0.0.0-00010101000000-000000000000
	github.com/mochi-mqtt/server/v2 v2.7.7
	google.golang.org/grpc v1.71.0
)

require (
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/rs/xid v1.4.0 // indirect
	golang.org/x/net v0.37.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250313205543-e70fdf4c4cb4 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)
