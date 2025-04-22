module github.com/cc-0000/indeq/desktop

go 1.23.5

replace github.com/cc-0000/indeq/common => ../common

require (
	github.com/cc-0000/indeq/common v0.0.0-00010101000000-000000000000
	github.com/eclipse/paho.mqtt.golang v1.5.0
	github.com/lib/pq v1.10.9
	github.com/segmentio/kafka-go v0.4.47
	google.golang.org/grpc v1.71.0
	google.golang.org/protobuf v1.36.6
)

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/klauspost/compress v1.17.5 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/stretchr/testify v1.8.4 // indirect
	golang.org/x/net v0.37.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250313205543-e70fdf4c4cb4 // indirect
)
