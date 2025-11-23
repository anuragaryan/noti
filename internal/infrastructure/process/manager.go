// Package process provides process lifecycle management utilities
package process

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Manager manages external process lifecycle
type Manager struct {
	cmd     *exec.Cmd
	running bool
	mutex   sync.Mutex
}

// NewManager creates a new process manager
func NewManager() *Manager {
	return &Manager{}
}

// Start starts the process with the given command and arguments
func (m *Manager) Start(command string, args ...string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return fmt.Errorf("process is already running")
	}

	// Create command
	m.cmd = exec.Command(command, args...)

	// Start the process
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	m.running = true
	return nil
}

// StartWithOutput starts the process and captures stdout/stderr
func (m *Manager) StartWithOutput(command string, args ...string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return fmt.Errorf("process is already running")
	}

	// Create command
	m.cmd = exec.Command(command, args...)
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr

	// Start the process
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	m.running = true
	return nil
}

// Stop stops the process gracefully with a timeout
func (m *Manager) Stop(timeout time.Duration) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running || m.cmd == nil {
		return nil
	}

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
	case <-time.After(timeout):
		// Force kill if not stopped within timeout
		if m.cmd.Process != nil {
			m.cmd.Process.Kill()
		}
	case <-done:
		// Process exited
	}

	m.running = false
	m.cmd = nil
	return nil
}

// IsRunning returns whether the process is running
func (m *Manager) IsRunning() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.running
}

// GetPID returns the process ID if running
func (m *Manager) GetPID() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running && m.cmd != nil && m.cmd.Process != nil {
		return m.cmd.Process.Pid
	}
	return 0
}

// WaitForExit waits for the process to exit
func (m *Manager) WaitForExit() error {
	m.mutex.Lock()
	cmd := m.cmd
	m.mutex.Unlock()

	if cmd == nil {
		return fmt.Errorf("no process running")
	}

	return cmd.Wait()
}
