package util

import (
	"crypto/rand"
	"fmt"
)

// func() (string, error)
//   - generates a secure 6-digit numeric OTP using crypto/rand
//   - avoids modulo bias by discarding bytes > 250
//   - returns: the OTP string and error (if random generation fails)
func GenerateOTP() (string, error) {
	const digits = "0123456789"
	const length = 6
	otp := make([]byte, 0, length)
	// Use 250 as the maximum to avoid modulo bias
	max := byte(250)
	// buffer to read random bytes in batches
	buf := make([]byte, 16)
	for len(otp) < length {
		_, err := rand.Read(buf)
		if err != nil {
			return "", fmt.Errorf("failed to generate OTP: %w", err)
		}
		for _, b := range buf {
			if b > max {
				continue
			}
			otp = append(otp, digits[b%10])
			if len(otp) == length {
				break
			}
		}
	}

	return string(otp), nil
}
