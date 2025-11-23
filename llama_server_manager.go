package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// LlamaServerManager manages the llama-server sidecar process
type LlamaServerManager struct {
	basePath       string
	binaryPath     string
	modelPath      string
	port           int
	cmd            *exec.Cmd
	mutex          sync.Mutex
	running        bool
	downloadScript []byte
}

// NewLlamaServerManager creates a new llama-server manager
func NewLlamaServerManager(basePath string, downloadScript []byte) *LlamaServerManager {
	return &LlamaServerManager{
		basePath:       basePath,
		port:           51337, // Hardcoded port for llama-server
		downloadScript: downloadScript,
	}
}

// EnsureBinary ensures the llama-server binary is available
func (m *LlamaServerManager) EnsureBinary() error {
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

	// Write download script
	scriptPath := filepath.Join(binDir, ".download-llama-server.sh")
	if err := os.WriteFile(scriptPath, m.downloadScript, 0755); err != nil {
		return fmt.Errorf("failed to write download script: %w", err)
	}
	defer os.Remove(scriptPath)

	// Run download script
	fmt.Println("Running llama-server download script...")
	cmd := exec.Command("/bin/bash", scriptPath)
	cmd.Dir = binDir
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()

	// Always print output for debugging
	fmt.Printf("Download script output:\n%s\n", string(output))

	if err != nil {
		return fmt.Errorf("failed to download llama-server: %w\nOutput: %s", err, string(output))
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
func (m *LlamaServerManager) Start(modelPath string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
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

	// Create command
	m.cmd = exec.Command(m.binaryPath, args...)

	// Capture output for debugging
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr

	// Start the process
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start llama-server: %w", err)
	}

	m.running = true
	fmt.Printf("✓ llama-server started (PID: %d)\n", m.cmd.Process.Pid)

	// Wait for server to be ready
	if err := m.WaitForReady(30 * time.Second); err != nil {
		m.Stop()
		return fmt.Errorf("llama-server failed to become ready: %w", err)
	}

	fmt.Println("✓ llama-server is ready!")
	return nil
}

// WaitForReady waits for the server to be ready
func (m *LlamaServerManager) WaitForReady(timeout time.Duration) error {
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
func (m *LlamaServerManager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running || m.cmd == nil {
		return nil
	}

	fmt.Println("Stopping llama-server...")

	// Try graceful shutdown first
	if m.cmd.Process != nil {
		if err := m.cmd.Process.Signal(os.Interrupt); err != nil {
			// If interrupt fails, force kill
			m.cmd.Process.Kill()
		}
	}

	// Wait for process to exit
	done := make(chan error, 1)
	go func() {
		done <- m.cmd.Wait()
	}()

	select {
	case <-time.After(5 * time.Second):
		// Force kill if not stopped within 5 seconds
		if m.cmd.Process != nil {
			m.cmd.Process.Kill()
		}
	case <-done:
		// Process exited
	}

	m.running = false
	m.cmd = nil
	fmt.Println("✓ llama-server stopped")
	return nil
}

// IsRunning returns whether the server is running
func (m *LlamaServerManager) IsRunning() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.running
}

// GetEndpoint returns the server endpoint URL
func (m *LlamaServerManager) GetEndpoint() string {
	return fmt.Sprintf("http://127.0.0.1:%d", m.port)
}

// HealthCheck performs a health check on the server
func (m *LlamaServerManager) HealthCheck() error {
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
func (m *LlamaServerManager) GetModelPath() string {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.modelPath
}
