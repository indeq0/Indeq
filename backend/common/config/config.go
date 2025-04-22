package config

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

func LoadSharedConfig() error {
	envPath := ".env"
	return godotenv.Load(envPath)
}

func LoadServerTLSFromEnv(certEnvName string, keyEnvName string) (*tls.Config, error) {
	// Decode cert and key from env vars
	certPEM, err := base64.StdEncoding.DecodeString(os.Getenv(certEnvName))
	if err != nil {
		return nil, fmt.Errorf("failed to decode cert: %v", err)
	}

	keyPEM, err := base64.StdEncoding.DecodeString(os.Getenv(keyEnvName))
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %v", err)
	}

	// Load the key pair
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %v", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

func LoadClientTLSFromEnv(certEnvName string, keyEnvName string, cacrtEnvName string) (*tls.Config, error) {
	certPEM, err := base64.StdEncoding.DecodeString(os.Getenv(certEnvName))
	if err != nil {
		return nil, fmt.Errorf("failed to decode cert: %v", err)
	}

	keyPEM, err := base64.StdEncoding.DecodeString(os.Getenv(keyEnvName))
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %v", err)
	}

	cacrtPEM, err := base64.StdEncoding.DecodeString(os.Getenv(cacrtEnvName))
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %v", err)
	}

	// Import client certificate/key pair
	clientCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %v", err)
	}

	// Add CA to cert pool
	certpool := x509.NewCertPool()
	if ok := certpool.AppendCertsFromPEM(cacrtPEM); !ok {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	// Create TLS configuration
	tlsConfig := &tls.Config{
		RootCAs:            certpool,
		Certificates:       []tls.Certificate{clientCert},
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: false,
	}

	return tlsConfig, nil
}

func LoadMTLSFromEnv(certEnvName string, keyEnvName string, cacrtEnvName string) (*tls.Config, error) {
	certpool := x509.NewCertPool()

	certPEM, err := base64.StdEncoding.DecodeString(os.Getenv(certEnvName))
	if err != nil {
		return nil, fmt.Errorf("failed to decode cert: %v", err)
	}

	keyPEM, err := base64.StdEncoding.DecodeString(os.Getenv(keyEnvName))
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %v", err)
	}

	cacrtPEM, err := base64.StdEncoding.DecodeString(os.Getenv(cacrtEnvName))
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %v", err)
	}

	// Add CA to cert pool
	if ok := certpool.AppendCertsFromPEM(cacrtPEM); !ok {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	// Import client certificate/key pair
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %v", err)
	}

	// Create TLS configuration
	tlsConfig := &tls.Config{
		ClientCAs:          certpool,
		Certificates:       []tls.Certificate{cert},
		MinVersion:         tls.VersionTLS13,
		ClientAuth:         tls.RequireAndVerifyClientCert,
		InsecureSkipVerify: false,
	}

	return tlsConfig, nil
}
