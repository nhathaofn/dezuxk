package services

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"dezuxk/internal/config"
	"dezuxk/internal/models"
)

// GatewayService manages the lifecycle of the API Gateway HTTP server.
type GatewayService struct {
	mu        sync.RWMutex
	server    *http.Server
	isRunning bool
	ip        string
	port      int
	bind      string
}

// NewGatewayService creates a new GatewayService instance.
func NewGatewayService(defaultPort int) *GatewayService {
	if defaultPort <= 0 {
		defaultPort = config.DefaultGatewayPort
	}
	return &GatewayService{
		port: defaultPort,
		ip:   gatewayBindAddress(),
		bind: gatewayBindAddress(),
	}
}

// DetectLocalIP detects the machine's primary local LAN IPv4 address.
func DetectLocalIP() string {
	// Method 1: Dial UDP to determine the preferred local outbound interface
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 800*time.Millisecond)
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		if localAddr.IP != nil && !localAddr.IP.IsLoopback() && localAddr.IP.To4() != nil {
			return localAddr.IP.String()
		}
	}

	// Method 2: Scan network interfaces for active non-loopback IPv4
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			// Skip down or loopback interfaces
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
					return ip.String()
				}
			}
		}
	}

	return "127.0.0.1"
}

// GetStatus returns the current running status, IP, and port of the gateway.
func (s *GatewayService) GetStatus() *models.GatewayStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ip := s.ip
	if s.isRunning {
		ip = s.bind
	}

	return &models.GatewayStatus{
		IsRunning: s.isRunning,
		IP:        ip,
		Port:      s.port,
	}
}

// GetPort returns the currently configured port.
func (s *GatewayService) GetPort() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.port
}

// SetPort updates the port and restarts the server if it is currently running.
func (s *GatewayService) SetPort(port int) (*models.GatewayStatus, error) {
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("cổng không hợp lệ (phải từ 1 đến 65535)")
	}

	s.mu.Lock()
	wasRunning := s.isRunning
	s.port = port
	s.mu.Unlock()

	if wasRunning {
		if _, err := s.Stop(); err != nil {
			log.Printf("[Gateway] Warning stopping server for port change: %v", err)
		}
		return s.Start(port)
	}

	return s.GetStatus(), nil
}

// Start launches the gateway HTTP server on the given port.
func (s *GatewayService) Start(port int) (*models.GatewayStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return &models.GatewayStatus{
			IsRunning: true,
			IP:        s.ip,
			Port:      s.port,
		}, nil
	}

	if port > 0 {
		s.port = port
	}
	s.ip = s.bind

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"online","gateway":"Gateway Manager","time":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})

	addr := fmt.Sprintf("%s:%d", s.bind, s.port)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("không thể lắng nghe cổng %d: %w", s.port, err)
	}

	s.server = server
	s.isRunning = true

	go func() {
		log.Printf("[Gateway] Server running at http://%s:%d\n", s.ip, s.port)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[Gateway] Server error: %v\n", err)
			s.mu.Lock()
			s.isRunning = false
			s.mu.Unlock()
		}
	}()

	return &models.GatewayStatus{
		IsRunning: true,
		IP:        s.ip,
		Port:      s.port,
	}, nil
}

func gatewayBindAddress() string {
	if bind := os.Getenv("GATEWAY_BIND_ADDRESS"); bind != "" {
		return bind
	}
	return config.DefaultGatewayBind
}

// Stop gracefully terminates the running gateway HTTP server.
func (s *GatewayService) Stop() (*models.GatewayStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning || s.server == nil {
		s.isRunning = false
		return &models.GatewayStatus{
			IsRunning: false,
			IP:        s.ip,
			Port:      s.port,
		}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := s.server.Shutdown(ctx)
	s.server = nil
	s.isRunning = false

	log.Println("[Gateway] Server stopped.")

	return &models.GatewayStatus{
		IsRunning: false,
		IP:        s.ip,
		Port:      s.port,
	}, err
}

// Toggle starts the server if stopped, or stops it if running.
func (s *GatewayService) Toggle() (*models.GatewayStatus, error) {
	s.mu.RLock()
	running := s.isRunning
	port := s.port
	s.mu.RUnlock()

	if running {
		return s.Stop()
	}
	return s.Start(port)
}
