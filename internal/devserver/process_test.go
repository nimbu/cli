package devserver

import "testing"

func TestProcessStartRejectsWhenAlreadyRunning(t *testing.T) {
	p := NewProcess(ChildConfig{Command: "echo"})
	p.running = true

	err := p.Start()
	if err == nil {
		t.Fatal("expected already running error")
	}
}
