FROM python:3.12-slim

WORKDIR /app

# Copy requirements and install dependencies
COPY embedding/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy the proto file
COPY common/api/embedding.proto /app/protos/embedding.proto
COPY common/config/.env .

# Generate Python code from proto
RUN python -m grpc_tools.protoc -I./protos --python_out=. --grpc_python_out=. ./protos/embedding.proto

# Copy the server implementation
COPY embedding/server.py .

# Expose the gRPC port
EXPOSE 50057

# Run the server
CMD ["python", "server.py"]
