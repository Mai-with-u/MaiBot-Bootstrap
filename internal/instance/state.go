package instance

import "fmt"

const (
	StateInstalled = "installed"
	StateRunning   = "running"
	StateStopped   = "stopped"
	StateUpdating  = "updating"
	StateFailed    = "failed"
)

var allowedTransitions = map[string]map[string]bool{
	"": {
		StateInstalled: true,
	},
	StateInstalled: {
		StateInstalled: true,
		StateRunning:   true,
		StateUpdating:  true,
		StateFailed:    true,
		StateStopped:   true,
	},
	StateRunning: {
		StateRunning:  true,
		StateStopped:  true,
		StateUpdating: true,
		StateFailed:   true,
	},
	StateStopped: {
		StateStopped:  true,
		StateRunning:  true,
		StateUpdating: true,
		StateFailed:   true,
	},
	StateUpdating: {
		StateUpdating:  true,
		StateInstalled: true,
		StateRunning:   true,
		StateStopped:   true,
		StateFailed:    true,
	},
	StateFailed: {
		StateFailed:    true,
		StateInstalled: true,
		StateUpdating:  true,
		StateStopped:   true,
	},
}

func ValidateTransition(from, to string) error {
	if to == "" {
		return fmt.Errorf("target state must not be empty")
	}
	next, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("unknown state: %q", from)
	}
	if !next[to] {
		return fmt.Errorf("invalid state transition: %q -> %q", from, to)
	}
	return nil
}
