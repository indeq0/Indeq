# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache protobuf-dev git make
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest 
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Copy module files first
COPY waitlist/go.mod waitlist/go.sum ./waitlist/
COPY common/go.mod common/go.sum ./common/

# Download dependencies
RUN cd waitlist && go mod download
RUN cd common && go mod download

# Copy source code
COPY waitlist/ ./waitlist/
COPY common/ ./common/

# Create the proto files if they aren't already generated
RUN cd common && make gen

# Build binary with module replacement
RUN cd waitlist && \
    CGO_ENABLED=0 GOOS=linux go build -o ../waitlist-service .

# Runtime stage  
FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /app/
COPY --from=builder /app/waitlist-service .
COPY --from=builder /app/common/config/.env . 

EXPOSE 50055
CMD ["./waitlist-service"]
