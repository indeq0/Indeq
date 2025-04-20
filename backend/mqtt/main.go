package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cc-0000/indeq/common/config"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
)

func main() {
	// Load .env variables
	err := config.LoadSharedConfig()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	// Configure TLS
	tlsConfig, err := config.LoadMTLSFromEnv("MQTT_CRT", "MQTT_KEY", "CA_CRT")
	if err != nil {
		log.Fatalf("Error loading TLS config for mqtt service: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create a new MQTT Server instance
	server := mqtt.New(nil)
	defer server.Close()

	// Add the auth hook to allow connections
	certHook := NewCertAuthHook()
	err = server.AddHook(certHook, nil)
	if err != nil {
		log.Fatalf("failed to add certificate hook to mqtt server: %v", err)
	}

	// Create TLS listener on the env port
	mqttPort, ok := os.LookupEnv("MQTT_PORT")
	if !ok {
		log.Fatal("failed to retrieve the mqtt port value from .env")
	}
	tlsTCP := listeners.NewTCP(listeners.Config{
		ID:        "ssl1",
		Address:   mqttPort,
		TLSConfig: tlsConfig,
	})

	err = server.AddListener(tlsTCP)
	if err != nil {
		log.Fatalf("failed to add TLS listener to mqtt server: %v", err)
	}

	go func() {
		err = server.Serve()
		if err != nil {
			log.Fatalf("encountered an error while serving the mqtt server: %v", err)
		}
	}()

	<-sigChan
}
