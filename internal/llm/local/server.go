package local

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"noti/internal/infrastructure/downloader"
	"noti/internal/infrastructure/process"
)

// ServerManager manages the llama-server lifecycle
type ServerManager struct {
	processManager *process.Manager
	downloader     *downloader.Downloader
	basePath       string
	binaryPath     string
	modelPath      string
	port           int
	mutex          sync.Mutex
}

// NewServerManager creates a new server manager
func NewServerManager(basePath string, downloadScript []byte) *ServerManager {
	return &ServerManager{
		processManager: process.NewManager(),
		downloader:     downloader.NewDownloader(downloadScript),
		basePath:       basePath,
		port:           51337, // Hardcoded port for llama-server
	}
}

// EnsureBinary ensures the llama-server binary is available
func (m *ServerManager) EnsureBinary() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Determine binary name based on platform
	binaryName := "llama-server"
	if runtime.GOOS == "windows" {
		binaryName = "llama-server.exe"
	}

	// Check in bin directory
	binPath := filepath.Join(m.basePath, "bin", binaryName)
	if _, err := os.Stat(binPath); err == nil {
		m.binaryPath = binPath
		fmt.Printf("✓ Found llama-server binary at: %s\n", binPath)
		return nil
	}

	fmt.Println("llama-server binary not found, downloading...")

	// Create bin directory
	binDir := filepath.Join(m.basePath, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Download using the downloader
	if err := m.downloader.Download(binDir); err != nil {
		return fmt.Errorf("failed to download llama-server: %w", err)
	}

	// Verify binary exists
	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("binary not found after download: %w", err)
	}

	m.binaryPath = binPath
	fmt.Printf("✓ llama-server binary downloaded to: %s\n", binPath)
	return nil
}

// Start starts the llama-server with the specified model
func (m *ServerManager) Start(modelPath string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.processManager.IsRunning() {
		return fmt.Errorf("llama-server is already running")
	}

	if m.binaryPath == "" {
		return fmt.Errorf("binary path not set, call EnsureBinary first")
	}

	if modelPath == "" {
		return fmt.Errorf("model path is required")
	}

	// Verify model exists
	if _, err := os.Stat(modelPath); err != nil {
		return fmt.Errorf("model file not found: %s", modelPath)
	}

	m.modelPath = modelPath

	// Build command arguments
	args := []string{
		"-m", modelPath,
		"--port", fmt.Sprintf("%d", m.port),
		"--host", "127.0.0.1",
		"-c", "2048", // context size
		"-n", "512", // max tokens to predict
		"--log-disable", // disable logging to reduce noise
	}

	fmt.Printf("Starting llama-server with model: %s\n", modelPath)
	fmt.Printf("Command: %s %v\n", m.binaryPath, args)

	// Start the process
	if err := m.processManager.StartWithOutput(m.binaryPath, args...); err != nil {
		return fmt.Errorf("failed to start llama-server: %w", err)
	}

	fmt.Printf("✓ llama-server started (PID: %d)\n", m.processManager.GetPID())

	// Wait for server to be ready
	if err := m.WaitForReady(30 * time.Second); err != nil {
		m.processManager.Stop(5 * time.Second)
		return fmt.Errorf("llama-server failed to become ready: %w", err)
	}

	fmt.Println("✓ llama-server is ready!")
	return nil
}

// WaitForReady waits for the server to be ready
func (m *ServerManager) WaitForReady(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", m.port)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for server to be ready")
		case <-ticker.C:
			resp, err := http.Get(healthURL)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
}

// Stop stops the llama-server process
func (m *ServerManager) Stop() error {
	fmt.Println("Stopping llama-server...")
	return m.processManager.Stop(5 * time.Second)
}

// IsRunning returns whether the server is running
func (m *ServerManager) IsRunning() bool {
	return m.processManager.IsRunning()
}

// GetEndpoint returns the server endpoint URL
func (m *ServerManager) GetEndpoint() string {
	return fmt.Sprintf("http://127.0.0.1:%d", m.port)
}

// HealthCheck performs a health check on the server
func (m *ServerManager) HealthCheck() error {
	if !m.IsRunning() {
		return fmt.Errorf("server is not running")
	}

	healthURL := fmt.Sprintf("%s/health", m.GetEndpoint())
	resp, err := http.Get(healthURL)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health check returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetModelPath returns the current model path
func (m *ServerManager) GetModelPath() string {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.modelPath
}
