package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

	"github.com/cc-0000/indeq/common/config"
	"github.com/go-kivik/kivik/v4"
	_ "github.com/go-kivik/kivik/v4/couchdb"
	"github.com/segmentio/kafka-go"
)

func createTopics(brokerAddress string, topics []kafka.TopicConfig) error {
	// Connect to broker
	connection, err := kafka.Dial("tcp", brokerAddress)
	if err != nil {
		return fmt.Errorf("failed to dial broker: %w", err)
	}
	defer connection.Close()

	// Get controller connection info
	controller, err := connection.Controller()
	if err != nil {
		return fmt.Errorf("failed to get controller: %w", err)
	}

	// Connect to controller broker using that info
	controllerConn, err := kafka.Dial("tcp",
		net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return fmt.Errorf("failed to dial controller: %w", err)
	}
	defer controllerConn.Close()

	if err := controllerConn.CreateTopics(topics...); err != nil {
		return fmt.Errorf("failed to create topics: %w", err)
	}

	return nil
}

// Add to your main.go in the init service
func initCouchDBSystemDatabases() error {
	// Get credentials from env
	couchdbUser, ok := os.LookupEnv("COUCHDB_USER")
	if !ok {
		return fmt.Errorf("failed to retrieve the couchdb user")
	}
	couchdbPassword, ok := os.LookupEnv("COUCHDB_PASSWORD")
	if !ok {
		return fmt.Errorf("failed to retrieve the couchdb password")
	}
	couchdbAddress, ok := os.LookupEnv("COUCHDB_ADDRESS")
	if !ok {
		return fmt.Errorf("failed to retrieve the couchdb address")
	}

	// Connect to CouchDB
	client, err := kivik.New("couch", fmt.Sprintf("http://%s:%s@%s/",
		couchdbUser, couchdbPassword, couchdbAddress))
	if err != nil {
		return fmt.Errorf("failed to connect to couchdb: %v", err)
	}
	defer client.Close()

	// Create system databases if they don't exist
	systemDBs := []string{"_users", "_replicator", "_global_changes"}
	for _, dbName := range systemDBs {
		exists, err := client.DBExists(context.TODO(), dbName)
		if err != nil {
			return fmt.Errorf("failed to check if %s exists: %v", dbName, err)
		}
		if !exists {
			if err := client.CreateDB(context.TODO(), dbName); err != nil {
				return fmt.Errorf("failed to create %s database: %v", dbName, err)
			}
			log.Printf("Created CouchDB system database: %s", dbName)
		}
	}
	return nil
}

func main() {
	// Load .env variables
	err := config.LoadSharedConfig()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	// Load broker name from .env
	brokerAddress, ok := os.LookupEnv("KAFKA_BROKER_ADDRESS")
	if !ok {
		log.Fatal("failed to get retrieve broker address")
	}

	// Create Kafka topics
	topics := []kafka.TopicConfig{
		{
			Topic:             "text-chunks",
			NumPartitions:     10,
			ReplicationFactor: 1,
			ConfigEntries: []kafka.ConfigEntry{
				{
					ConfigName:  "retention.ms",
					ConfigValue: "86400000",
				},
				{
					ConfigName:  "cleanup.policy",
					ConfigValue: "delete",
				},
				{
					ConfigName:  "compression.type",
					ConfigValue: "lz4",
				},
			},
		},
		{
			Topic:             "desktop-signals",
			NumPartitions:     5,
			ReplicationFactor: 1,
			ConfigEntries: []kafka.ConfigEntry{
				{
					ConfigName:  "retention.ms",
					ConfigValue: "86400000",
				},
				{
					ConfigName:  "cleanup.policy",
					ConfigValue: "delete",
				},
				{
					ConfigName:  "compression.type",
					ConfigValue: "lz4",
				},
			},
		},
		{
			Topic:             "google-crawling-signals",
			NumPartitions:     5,
			ReplicationFactor: 1,
			ConfigEntries: []kafka.ConfigEntry{
				{
					ConfigName:  "retention.ms",
					ConfigValue: "86400000",
				},
				{
					ConfigName:  "cleanup.policy",
					ConfigValue: "delete",
				},
				{
					ConfigName:  "compression.type",
					ConfigValue: "lz4",
				},
			},
		},
		{
			Topic:             "notion-crawling-signals",
			NumPartitions:     5,
			ReplicationFactor: 1,
			ConfigEntries: []kafka.ConfigEntry{
				{
					ConfigName:  "retention.ms",
					ConfigValue: "86400000",
				},
				{
					ConfigName:  "cleanup.policy",
					ConfigValue: "delete",
				},
				{
					ConfigName:  "compression.type",
					ConfigValue: "lz4",
				},
			},
		},
		{
			Topic:             "microsoft-crawling-signals",
			NumPartitions:     5,
			ReplicationFactor: 1,
			ConfigEntries: []kafka.ConfigEntry{
				{
					ConfigName:  "retention.ms",
					ConfigValue: "86400000",
				},
				{
					ConfigName:  "cleanup.policy",
					ConfigValue: "delete",
				},
				{
					ConfigName:  "compression.type",
					ConfigValue: "lz4",
				},
			},
		},
	}
	err = createTopics(brokerAddress, topics)
	if err != nil {
		log.Fatalf("Failed to create topics: %v", err)
	}

	log.Print("Created topics successfully!")

	// Initialize CouchDB system databases
	err = initCouchDBSystemDatabases()
	if err != nil {
		log.Fatalf("Failed to initialize CouchDB system databases: %v", err)
	}

	log.Print("Initialized CouchDB system databases successfully!")

	// Other init routines go here:
}
