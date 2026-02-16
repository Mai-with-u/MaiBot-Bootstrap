//go:build !windows

package execx

import "os"

func isRoot() bool {
	return os.Geteuid() == 0
}
