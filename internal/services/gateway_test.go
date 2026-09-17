package services_test

import (
	"testing"

	"dezuxk/internal/services"
)

func TestGatewayService(t *testing.T) {
	ip := services.DetectLocalIP()
	if ip == "" {
		t.Fatalf("Expected detected IP, got empty string")
	}

	gateway := services.NewGatewayService(9099)
	status := gateway.GetStatus()
	if status.IsRunning {
		t.Fatalf("Expected gateway to be stopped initially, got running")
	}

	// Start gateway on test port
	startedStatus, err := gateway.Start(9099)
	if err != nil {
		t.Fatalf("Failed to start gateway: %v", err)
	}
	if !startedStatus.IsRunning {
		t.Fatalf("Expected gateway to be running")
	}
	if startedStatus.Port != 9099 {
		t.Errorf("Expected port 9099, got %d", startedStatus.Port)
	}

	// Stop gateway
	stoppedStatus, err := gateway.Stop()
	if err != nil {
		t.Fatalf("Failed to stop gateway: %v", err)
	}
	if stoppedStatus.IsRunning {
		t.Fatalf("Expected gateway to be stopped")
	}
}
