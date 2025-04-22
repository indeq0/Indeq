module github.com/cc-0000/indeq/init

go 1.23.5

replace github.com/cc-0000/indeq/common => ../common

require (
	github.com/cc-0000/indeq/common v0.0.0-00010101000000-000000000000
	github.com/go-kivik/kivik/v4 v4.3.3
	github.com/segmentio/kafka-go v0.4.47
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	golang.org/x/net v0.37.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
)
