package services

import (
	"fmt"
	"net"
	"testing"
)

// Start used to launch ListenAndServe in a goroutine and return nil regardless,
// so a port that was already taken still reported the webhook as listening.
func TestWebhookStartReportsBindFailure(t *testing.T) {
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer blocker.Close()
	port := blocker.Addr().(*net.TCPAddr).Port

	// Occupy the same port on every interface so the service cannot bind it.
	wide, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err == nil {
		defer wide.Close()
	}

	svc := NewWebhookService(nil)
	if err := svc.Start(port); err == nil {
		t.Fatal("Start reported success on an occupied port")
	}
	if svc.IsRunning() {
		t.Fatal("service claims to be running after a failed bind")
	}
}

func TestWebhookStartStopOnFreePort(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	svc := NewWebhookService(nil)
	if err := svc.Start(port); err != nil {
		t.Fatalf("Start on a free port: %v", err)
	}
	if !svc.IsRunning() {
		t.Fatal("service should be running after a successful start")
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if svc.IsRunning() {
		t.Fatal("service still reports running after Stop")
	}
}
