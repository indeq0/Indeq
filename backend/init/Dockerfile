# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache protobuf-dev git make
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest 
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Copy module files first
COPY init/go.mod init/go.sum ./init/
COPY common/go.mod common/go.sum ./common/

# Download dependencies
RUN cd init && go mod download
RUN cd common && go mod download

# Copy source code
COPY init/ ./init/
COPY common/ ./common/

# Create the proto files if they aren't already generated
RUN cd common && make gen

# Build binary with module replacement
RUN cd init && \
    CGO_ENABLED=0 GOOS=linux go build -o ../init-routine .

# Runtime stage  
FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /app/
COPY --from=builder /app/init-routine .
COPY --from=builder /app/common/config/.env . 

CMD ["./init-routine"]
