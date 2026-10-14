// Package client implements the TL-ER6220T SLP management protocol.
package client

import (
	"errors"
	"time"

	"github.com/joho/godotenv"
)

// Config contains transport and login settings. Credentials never become Terraform state.
type Config struct {
	Endpoint       string
	Username       string
	Password       string
	Insecure       bool
	AllowLegacyTLS bool
	ReadOnly       bool
	Timeout        time.Duration
}

// LoadEnvFile parses a dotenv file without changing the process environment.
// In particular, Windows' built-in USERNAME environment variable is not used.
func LoadEnvFile(path string) (map[string]string, error) {
	values, err := godotenv.Read(path)
	if err != nil {
		// Parser errors can include the malformed line, including its secret value.
		return nil, errors.New("unable to read or parse the credentials dotenv file")
	}
	return values, nil
}
