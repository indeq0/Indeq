package util

import (
	"crypto/rand"
	"fmt"
)

// GenerateCode generates a secure 6-character OTP.
// Accepts mode: "numeric" or "alphanumeric".
// Uses crypto/rand and avoids modulo bias.
func GenerateCode(mode string) (string, error) {
	var charset string
	switch mode {
	case "numeric":
		charset = "0123456789"
	case "alphanumeric":
		charset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	default:
		return "", fmt.Errorf("invalid mode: %s", mode)
	}

	const otpLength = 6
	const maxByte = 250 // to avoid modulo bias
	otp := make([]byte, 0, otpLength)
	buf := make([]byte, 16)

	for len(otp) < otpLength {
		_, err := rand.Read(buf)
		if err != nil {
			return "", fmt.Errorf("failed to generate OTP: %w", err)
		}
		for _, b := range buf {
			if b > maxByte {
				continue
			}
			otp = append(otp, charset[b%byte(len(charset))])
			if len(otp) == otpLength {
				break
			}
		}
	}

	return string(otp), nil
}
