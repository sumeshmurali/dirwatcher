// helpers contains utility functions or helper functions
package internal

import (
	"os"

	"github.com/joho/godotenv"
)

// Loads the env file to the enviorment if running from local
func LoadEnv() {
	if os.Getenv("ENV") == "local" {
		godotenv.Load()
	}
}
