//go:build windows

package execx

import (
	"os"
	"strings"
)

func isRoot() bool {
	user := strings.ToLower(os.Getenv("USERNAME"))
	if user == "administrator" {
		return true
	}
	return os.Getenv("MAIBOT_ASSUME_ADMIN") == "1"
}
