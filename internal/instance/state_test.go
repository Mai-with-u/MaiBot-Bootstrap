package instance

import "testing"

func TestValidateTransition(t *testing.T) {
	if err := ValidateTransition(StateInstalled, StateRunning); err != nil {
		t.Fatalf("expected valid transition, got error: %v", err)
	}
	if err := ValidateTransition(StateRunning, StateInstalled); err == nil {
		t.Fatalf("expected invalid transition error")
	}
}
