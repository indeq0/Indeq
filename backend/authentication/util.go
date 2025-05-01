package main

import (
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"golang.org/x/crypto/argon2"
)

// func(password string, argonParams *params) (string, error)
//   - generates a secure password hash using the given Argon2 params
//   - returns: the hashed password and error (if hashing fails)
func saltAndHashPassword(password string, argonParams *params) (string, error) {
	// Generate a random salt
	salt := make([]byte, argonParams.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("couldn't make a salt: %v", err)
	}

	// Generate a password hash
	hash := argon2.IDKey(
		[]byte(password),
		salt,
		argonParams.iterations,
		argonParams.memory,
		argonParams.parallelism,
		argonParams.keyLength,
	)

	// Keep encryption details alongside the hash
	encodedHash := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonParams.memory,
		argonParams.iterations,
		argonParams.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)

	return encodedHash, nil
}

// func(password string, hashed password to compare to)
//   - hashes the incoming password using the same settings as the encoded hash and compares them
//   - returns: (whether or not the password is right, any error)
func comparePasswordAndEncodedHash(password string, encodedHash string) (bool, error) {
	// Unencode the configuration variables from the password hash and salt
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid hash format")
	}

	// Get the version
	var version int
	_, err := fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return false, err
	}

	// Get the memory constraint, number of iterations, and amnt of parallelism
	var memory uint32
	var iterations uint32
	var parallelism uint8
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		return false, err
	}

	// Get the salt
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}

	// Get the hash
	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}

	// Compute the hash of the incoming password
	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		iterations,
		memory,
		parallelism,
		uint32(len(decodedHash)),
	)

	// Constant-time comparison
	return subtle.ConstantTimeCompare(computedHash, decodedHash) == 1, nil
}

// func(to string, subject string, body string)
//   - sends an email to the given address with the given subject and body
//   - returns an error if the email fails to send
func (s *authServer) sendEmail(to string, subject string, body string) error {
	from := os.Getenv("SMTP_FROM")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")

	// check if all the env variables are set
	if from == "" || user == "" || pass == "" || host == "" || port == "" {
		return fmt.Errorf("SMTP configuration is incomplete")
	}

	// set up the address
	addr := fmt.Sprintf("%s:%s", host, port)

	// set up the TLS configuration
	tlsConfig := &tls.Config{
		ServerName: host,
	}

	// dial the TLS connection
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to dial TLS connection: %w", err)
	}
	defer conn.Close()

	// create the SMTP client
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// set up the authentication
	auth := smtp.PlainAuth("", user, pass, host)
	if ok, _ := client.Extension("AUTH"); ok {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("failed to authenticate: %w", err)
		}
	}

	// set the sender
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// set the recipient
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("failed to set recipient: %w", err)
	}

	// start the data command
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to start data command: %w", err)
	}

	// set up the message
	msg := []byte("From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
		"\r\n" +
		body + "\r\n")

	// write the message
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// close the data writer
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	// quit the client
	if err := client.Quit(); err != nil {
		return fmt.Errorf("failed to quit SMTP client: %w", err)
	}

	return nil
}

// func(password string) error
//   - makes sure the password is within the configured min/max lengths
//   - assumes: parameters are loaded into memory already (via loadPasswordSettings() or otherwise)
func (s *authServer) validatePassword(password string) error {
	if len(password) < s.MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", s.MinPasswordLength)
	}
	if len(password) > s.MaxPasswordLength {
		return fmt.Errorf("password must not exceed %d characters", s.MaxPasswordLength)
	}
	return nil
}

// func(email string)
//   - checks to make sure the email is: within configured min/max lengths & formatted via RFC 5322
func (s *authServer) validateEmail(email string) error {
	if len(email) > s.MaxEmailLength {
		return fmt.Errorf("email must not exceed %d characters", s.MaxEmailLength)
	}
	if len(email) <= 0 {
		return fmt.Errorf("email must not be blank")
	}

	// Check email format
	_, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil {
		return fmt.Errorf("invalid email format: %w", err)
	}

	return nil
}

// TODO: implement name validation here
func (s *authServer) validateName(name string) error {
	return nil
}


