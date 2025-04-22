#!/bin/bash

# Create .env files in the specified directories
echo "Creating .env files..."

# Define the directories
declare -a dirs=("backend/gateway" "backend/authentication" "backend/query" "backend/common/config")

# Loop through each directory and create the .env file
for dir in "${dirs[@]}"; do
    echo "Creating .env in $dir"
    {
        echo "AUTH_PORT=${AUTH_PORT}"
        echo "AUTH_ADDRESS=${AUTH_ADDRESS}"
        echo "QUERY_PORT=${QUERY_PORT}"
        echo "QUERY_ADDRESS=${QUERY_ADDRESS}"
        echo "VECTOR_PORT=${VECTOR_PORT}"
        echo "VECTOR_ADDRESS=${VECTOR_ADDRESS}"
        echo "GATEWAY_ADDRESS=${GATEWAY_ADDRESS}"

        # Database parameters
        echo "DATABASE_URL=${DATABASE_URL}"
        echo "POSTGRES_USER=${POSTGRES_USER}"
        echo "POSTGRES_PASSWORD=${POSTGRES_PASSWORD}"
        echo "POSTGRES_DB=${POSTGRES_DB}"

        echo "JWT_SECRET=${JWT_SECRET}"

        # Argon2 Parameters
        echo "ARGON2_MEMORY=${ARGON2_MEMORY}"
        echo "ARGON2_ITERATIONS=${ARGON2_ITERATIONS}"
        echo "ARGON2_PARALLELISM=${ARGON2_PARALLELISM}"
        echo "ARGON2_SALT_LENGTH=${ARGON2_SALT_LENGTH}"
        echo "ARGON2_KEY_LENGTH=${ARGON2_KEY_LENGTH}"

        # Password and Email Constraints
        echo "MIN_PASSWORD_LENGTH=${MIN_PASSWORD_LENGTH}"
        echo "MAX_PASSWORD_LENGTH=${MAX_PASSWORD_LENGTH}"
        echo "MAX_EMAIL_LENGTH=${MAX_EMAIL_LENGTH}"

        # RabbitMQ parameters
        echo "RABBITMQ_URL=${RABBITMQ_URL}"
        echo "RABBITMQ_DEFAULT_USER=${RABBITMQ_DEFAULT_USER}"
        echo "RABBITMQ_DEFAULT_PASS=${RABBITMQ_DEFAULT_PASS}"
        echo "RABBITMQ_LOGS=${RABBITMQ_LOGS}"

        # Ollama parameters
        echo "OLLAMA_URL=${OLLAMA_URL}"
        echo "LLM_MODEL=${LLM_MODEL}"

        # Zilliz parameters
        echo "ZILLIZ_ADDRESS=${ZILLIZ_ADDRESS}"
        echo "ZILLIZ_API_KEY=${ZILLIZ_API_KEY}"

        # TLS parameters in base 64
        echo "CA_CRT=${CA_CRT}"
        echo "QUERY_CRT=${QUERY_CRT}"
        echo "QUERY_KEY=${QUERY_KEY}"
        echo "AUTH_CRT=${AUTH_CRT}"
        echo "AUTH_KEY=${AUTH_KEY}"
        echo "GATEWAY_CRT=${GATEWAY_CRT}"
        echo "GATEWAY_KEY=${GATEWAY_KEY}"

        # CORS parameters
        echo "ALLOWED_CLIENT_IP=${ALLOWED_CLIENT_IP}"
    } > "$dir/.env"
done

echo ".env files created successfully."