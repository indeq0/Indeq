package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log"
	"os"
	"sync"
	"time"

	pb "github.com/cc-0000/indeq/common/api"
	"github.com/cc-0000/indeq/common/config"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// struct(extends mqtt.HookBase class)
//   - overrides the authentication methods to provide custom authentication
//   - implements topic restrictions based on the connecting client's certificate UID
type CertAuthHook struct {
	mqtt.HookBase
	mu           sync.RWMutex
	clientIDtoUID map[string]string // Map of client IDs --> certificate UIDs
	desktopClient pb.DesktopServiceClient
	desktopConn   *grpc.ClientConn
}

// func()
//   - static constructor
//   - initializes the client to certificate uid map and connects to desktop service
func NewCertAuthHook() *CertAuthHook {
	hook := &CertAuthHook{
		clientIDtoUID: make(map[string]string),
	}

	// Connect to desktop service
	desktopAddy, ok := os.LookupEnv("DESKTOP_ADDRESS")
	if !ok {
		log.Printf("Warning: DESKTOP_ADDRESS environment variable not set, online status updates will be disabled")
		return hook
	}

	// Configure TLS for desktop service connection
	tlsConfig, err := config.LoadClientTLSFromEnv("MQTT_CRT", "MQTT_KEY", "CA_CRT")
	if err != nil {
		log.Printf("Warning: Failed to load TLS config for desktop service: %v, online status updates will be disabled", err)
		return hook
	}

	// Set up connection to desktop service with TLS
	conn, err := grpc.NewClient(desktopAddy,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		log.Printf("Warning: Failed to connect to desktop service: %v, online status updates will be disabled", err)
		return hook
	}

	hook.desktopConn = conn
	hook.desktopClient = pb.NewDesktopServiceClient(conn)
	return hook
}

// func()
//   - overrides the ID() method in mqtt.HookBase
//   - returns the ID of this hook
func (h *CertAuthHook) ID() string {
	return "cert-auth-hook"
}

// func(byte value of the capability that mqtt wants to check if we provide)
//   - overrides the Provides() method in mqtt.HookBase
//   - returns true if the byte matches one the enums that we implement
func (h *CertAuthHook) Provides(b byte) bool {
	return b == mqtt.OnConnectAuthenticate ||
		b == mqtt.OnACLCheck ||
		b == mqtt.OnDisconnect
}

// func(config interface)
//   - overrides the Init() method in mqtt.HookBase
//   - initializes the hook with configuration (not used in this implementation)
func (h *CertAuthHook) Init(config any) error {
	return nil
}

// func(client pointer, packet)
//   - overrides the OnConnectAuthenticate() method in mqtt.HookBase
//   - authenticates clients if they have the valid TLS certificates
//   - updates the user's online status to true in the desktop service
//   - maps the client ID to the client's embedded UID
func (h *CertAuthHook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	// Check if client has TLS connection and certificate
	tlsConn, ok := cl.Net.Conn.(*tls.Conn)
	if !ok {
		return false // Require TLS connection
	}

	// Extract client certificate
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return false // No client certificate provided
	}
	cert := state.PeerCertificates[0]

	// Extract UID from certificate
	uid := extractUIDFromCert(cert)
	if uid == "" {
		return false // No UID found in certificate
	}

	// Store the mapping between client ID and certificate UID in our hook
	h.mu.Lock()
	h.clientIDtoUID[cl.ID] = uid
	h.mu.Unlock()

	// Update user's online status to true in desktop service
	if h.desktopClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := h.desktopClient.UpdateUserOnlineStatus(ctx, &pb.UpdateUserOnlineStatusRequest{
			UserId:   uid,
			IsOnline: true,
		})

		if err != nil {
			log.Printf("Failed to update online status for user %s: %v", uid, err)
		}
	}

	return true
}

// func(client pointer, topic they are requesting access to, write or read access)
//   - overrides the OnACLCheck() method in mqtt.HookBase
//   - enforces access to only certain topics that end in the client's ID
//   - desktop-service should get unrestrained access
func (h *CertAuthHook) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	// Get stored UID from our mapping using client ID
	h.mu.RLock()
	uid, ok := h.clientIDtoUID[cl.ID]
	h.mu.RUnlock()
	if !ok {
		return false // No UID stored for this client
	}

	if uid == "desktop-service" {
		return true // desktop-service has unlimited permissions
	}

	// list of topics that clients are allowed to subscribe to:
	crawlReqTopic := "crawl_req/" + uid
	queryReqTopic := "query_req/" + uid
	newCrawlTopic := "new_crawl/" + uid
	newChunkTopic := "new_chunk/" + uid
	queryResTopic := "query_res/" + uid

	switch topic {
	case crawlReqTopic:
		// clients should only be able to READ crawl requests
		return !write
	case queryReqTopic:
		// clients should only be able to READ query requests
		return !write
	case newCrawlTopic:
		// clients should only be able to WRITE new crawls
		return write
	case newChunkTopic:
		// clients should only be able to WRITE new chunks
		return write
	case queryResTopic:
		// clients should only be able to WRITE query responses
		return write
	default:
		// they are requesting an invalid topic
		return false
	}
}

// func(certificate pointer)
//   - helper function to extract UID from x509 certificate
//   - extracts the UID from UID or CN (fallback) or returns an empty string if not found
func extractUIDFromCert(cert *x509.Certificate) string {
	// Look for UID in Subject
	var uid string
	var cn string
	for _, name := range cert.Subject.Names {
		if name.Type.String() == "0.9.2342.19200300.100.1.1" { // OID for UID
			if tmpUid, ok := name.Value.(string); ok {
				uid = tmpUid
			}
		} else if name.Type.String() == "2.5.4.3" { // OID for CN
			if tmpCn, ok := name.Value.(string); ok {
				cn = tmpCn
			}
		}
	}

	if uid != "" {
		return uid
	} else if cn != "" {
		return cn
	}

	return ""
}

// func(client pointer, error, boolean)
// - overrides the OnDisconnect() method in mqtt.HookBase
// - deletes the clientID to UID mapping when a client disconnects
// - updates the user's online status to false in the desktop service
func (h *CertAuthHook) OnDisconnect(cl *mqtt.Client, err error, expire bool) {
	// Get the UID before deleting the mapping
	h.mu.Lock()
	uid, ok := h.clientIDtoUID[cl.ID]
	if ok {
		delete(h.clientIDtoUID, cl.ID)
	}
	h.mu.Unlock()
	if !ok {
		return
	}

	// Update user's online status to false in desktop service
	if h.desktopClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := h.desktopClient.UpdateUserOnlineStatus(ctx, &pb.UpdateUserOnlineStatusRequest{
			UserId:   uid,
			IsOnline: false,
		})

		if err != nil {
			log.Printf("Failed to update offline status for user %s: %v", uid, err)
		}
	}
}

// func()
//   - overrides the OnStopped() method in mqtt.HookBase
//   - closes the connection to the desktop service
func (h *CertAuthHook) OnStopped() {
	if h.desktopConn != nil {
		h.desktopConn.Close()
	}
}
