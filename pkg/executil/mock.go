package executil

import (
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

	// Internals to support async streaming
	stdoutPipeReader *io.PipeReader
	stdoutPipeWriter *io.PipeWriter
	stderrPipeReader *io.PipeReader
	stderrPipeWriter *io.PipeWriter

	doneChan chan struct{}
}

// Run simulates executing the command.
func (m *MockCommand) Run() error {
	if err := m.Start(); err != nil {
		return err
	}
	return m.Wait()
}

// Start simulates starting the command asynchronously.
func (m *MockCommand) Start() error {
	m.doneChan = make(chan struct{})

	go func() {
		defer close(m.doneChan)

		var runErr error
		if m.RunFunc != nil {
			runErr = m.RunFunc()
		} else {
			runErr = m.RunErr
		}

		if m.stdoutPipeWriter != nil && len(m.OutData) > 0 {
			_, _ = m.stdoutPipeWriter.Write(m.OutData)
		}
		if m.stderrPipeWriter != nil && len(m.ErrData) > 0 {
			_, _ = m.stderrPipeWriter.Write(m.ErrData)
		}

		if m.Stdout != nil && len(m.OutData) > 0 {
			_, _ = m.Stdout.Write(m.OutData)
		}
		if m.Stderr != nil && len(m.ErrData) > 0 {
			_, _ = m.Stderr.Write(m.ErrData)
		}

		if m.stdoutPipeWriter != nil {
			_ = m.stdoutPipeWriter.CloseWithError(runErr)
		}
		if m.stderrPipeWriter != nil {
			_ = m.stderrPipeWriter.CloseWithError(runErr)
		}

		m.RunErr = runErr
	}()

	return nil
}

// Wait simulates waiting for the command to finish.
func (m *MockCommand) Wait() error {
	if m.doneChan != nil {
		<-m.doneChan
	}
	return m.RunErr
}

// StdoutPipe returns a pipe mock containing the predefined stdout data.
func (m *MockCommand) StdoutPipe() (io.ReadCloser, error) {
	if m.stdoutPipeReader != nil {
		return nil, fmt.Errorf("StdoutPipe already called")
	}
	r, w := io.Pipe()
	m.stdoutPipeReader = r
	m.stdoutPipeWriter = w
	return r, nil
}

// StderrPipe returns a pipe mock containing the predefined stderr data.
func (m *MockCommand) StderrPipe() (io.ReadCloser, error) {
	if m.stderrPipeReader != nil {
		return nil, fmt.Errorf("StderrPipe already called")
	}
	r, w := io.Pipe()
	m.stderrPipeReader = r
	m.stderrPipeWriter = w
	return r, nil
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
