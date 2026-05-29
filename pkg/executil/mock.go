package executil

import (
	"bytes"
	"fmt"
	"io"
)

// MockCommand implements Command for testing.
type MockCommand struct {
	Name    string
	Args    []string
	Env     []string
	Stdout  io.Writer
	Stderr  io.Writer
	RunErr  error
	OutData []byte
	ErrData []byte
	RunFunc func() error
}

// Run simulates executing the command.
func (m *MockCommand) Run() error {
	if m.RunFunc != nil {
		return m.RunFunc()
	}
	if m.Stdout != nil && len(m.OutData) > 0 {
		_, _ = m.Stdout.Write(m.OutData)
	}
	if m.Stderr != nil && len(m.ErrData) > 0 {
		_, _ = m.Stderr.Write(m.ErrData)
	}
	return m.RunErr
}

// Start simulates starting the command asynchronously.
func (m *MockCommand) Start() error {
	m.RunErr = m.Run()
	return nil
}

// Wait simulates waiting for the command to finish.
func (m *MockCommand) Wait() error {
	return m.RunErr
}

// StdoutPipe returns a pipe mock containing the predefined stdout data.
func (m *MockCommand) StdoutPipe() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m.OutData)), nil
}

// StderrPipe returns a pipe mock containing the predefined stderr data.
func (m *MockCommand) StderrPipe() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m.ErrData)), nil
}

// SetEnv sets command environment variables.
func (m *MockCommand) SetEnv(env []string) {
	m.Env = env
}

// SetStdout sets the standard output writer.
func (m *MockCommand) SetStdout(w io.Writer) {
	m.Stdout = w
}

// SetStderr sets the standard error writer.
func (m *MockCommand) SetStderr(w io.Writer) {
	m.Stderr = w
}

// MockExecutor implements Executor for testing.
type MockExecutor struct {
	Commands  []*MockCommand
	RunErr    error
	OutMap    map[string][]byte
	ErrMap    map[string][]byte
	PathMap   map[string]string
	OnCommand func(cmd *MockCommand)
}

// NewMockExecutor creates an initialized MockExecutor.
func NewMockExecutor() *MockExecutor {
	return &MockExecutor{
		OutMap:  make(map[string][]byte),
		ErrMap:  make(map[string][]byte),
		PathMap: make(map[string]string),
	}
}

// Command records and returns a MockCommand for testing.
func (e *MockExecutor) Command(name string, arg ...string) Command {
	cmd := &MockCommand{
		Name:    name,
		Args:    arg,
		RunErr:  e.RunErr,
		OutData: e.OutMap[name],
		ErrData: e.ErrMap[name],
	}
	e.Commands = append(e.Commands, cmd)
	if e.OnCommand != nil {
		e.OnCommand(cmd)
	}
	return cmd
}

// LookPath returns the mocked executable path or an error.
func (e *MockExecutor) LookPath(file string) (string, error) {
	if p, ok := e.PathMap[file]; ok {
		return p, nil
	}
	return "", fmt.Errorf("path not found for %s", file)
}
