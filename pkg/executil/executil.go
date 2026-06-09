package executil

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Command defines the interface for command execution wrappers.
type Command interface {
	Run() error
	Start() error
	Wait() error
	StdoutPipe() (io.ReadCloser, error)
	StderrPipe() (io.ReadCloser, error)
	SetEnv(env []string)
	SetStdout(w io.Writer)
	SetStderr(w io.Writer)
}

// Executor defines the interface to locate and create Command instances.
type Executor interface {
	Command(name string, arg ...string) Command
	LookPath(file string) (string, error)
}

// OSExecutor implements Executor using the standard os/exec package.
type OSExecutor struct{}

// NewOSExecutor creates a new OSExecutor.
func NewOSExecutor() *OSExecutor {
	return &OSExecutor{}
}

// startDbusSession starts a new transient dbus-daemon session in the background
// and returns its session bus address and PID.
func startDbusSession(lookPath func(string) (string, error)) (string, int, error) {
	dbusPath, err := lookPath("dbus-daemon")
	if err != nil {
		return "", 0, err
	}
	cmd := exec.Command(dbusPath, "--session", "--fork", "--print-address=1", "--print-pid=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", 0, err
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 2 {
		return "", 0, fmt.Errorf("unexpected dbus-daemon output: %q", out.String())
	}
	address := lines[0]
	var pid int
	if _, err := fmt.Sscanf(lines[1], "%d", &pid); err != nil {
		return "", 0, fmt.Errorf("failed to parse dbus-daemon PID from %q: %w", lines[1], err)
	}
	return address, pid, nil
}

// Command creates a new Command executing the given command and arguments on the OS.
func (e *OSExecutor) Command(name string, arg ...string) Command {
	return &osCommand{cmd: exec.Command(name, arg...)}
}

// LookPath searches for an executable binary in the system PATH.
func (e *OSExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

type osCommand struct {
	cmd     *exec.Cmd
	dbusPID int
}

func (c *osCommand) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

func (c *osCommand) Start() error {
	// Start D-Bus session if executing flatpak or flatpak-builder and no session is active.
	if c.dbusPID == 0 && (c.cmd.Path != "" && (strings.HasSuffix(c.cmd.Path, "flatpak") || strings.HasSuffix(c.cmd.Path, "flatpak-builder"))) {
		hasDbus := false
		for _, env := range c.cmd.Env {
			if strings.HasPrefix(env, "DBUS_SESSION_BUS_ADDRESS=") {
				hasDbus = true
				break
			}
		}
		if !hasDbus && os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
			if address, pid, err := startDbusSession(exec.LookPath); err == nil {
				c.dbusPID = pid
				if c.cmd.Env == nil {
					c.cmd.Env = append(os.Environ(), "DBUS_SESSION_BUS_ADDRESS="+address)
				} else {
					c.cmd.Env = append(c.cmd.Env, "DBUS_SESSION_BUS_ADDRESS="+address)
				}
			}
		}
	}
	return c.cmd.Start()
}

func (c *osCommand) Wait() error {
	err := c.cmd.Wait()
	if c.dbusPID > 0 {
		if proc, err := os.FindProcess(c.dbusPID); err == nil {
			_ = proc.Signal(syscall.SIGTERM)
		}
	}
	return err
}

func (c *osCommand) StdoutPipe() (io.ReadCloser, error) {
	return c.cmd.StdoutPipe()
}

func (c *osCommand) StderrPipe() (io.ReadCloser, error) {
	return c.cmd.StderrPipe()
}

func (c *osCommand) SetEnv(env []string) {
	c.cmd.Env = env
}

func (c *osCommand) SetStdout(w io.Writer) {
	c.cmd.Stdout = w
}

func (c *osCommand) SetStderr(w io.Writer) {
	c.cmd.Stderr = w
}

// StreamWithPrefix reads from r line-by-line and writes each line with a prefix to w.
// It uses bufio.Reader.ReadBytes('\n') to handle lines of arbitrary length efficiently without GC thrashing.
func StreamWithPrefix(r io.Reader, w io.Writer, prefix string) {
	reader := bufio.NewReader(r)
	var partial []byte
	for {
		chunk, err := reader.ReadSlice('\n')
		if len(chunk) > 0 {
			partial = append(partial, chunk...)
			if chunk[len(chunk)-1] == '\n' {
				line := bytes.TrimSuffix(partial, []byte("\n"))
				line = bytes.TrimSuffix(line, []byte("\r"))
				if prefix == "" {
					fmt.Fprintf(w, "%s\n", line)
				} else {
					fmt.Fprintf(w, "%s %s\n", prefix, line)
				}
				partial = partial[:0]
			} else if len(partial) >= 64*1024 {
				// Flush partial buffer if it grows too large to prevent unbounded memory growth
				line := bytes.TrimSuffix(partial, []byte("\r"))
				if prefix == "" {
					fmt.Fprintf(w, "%s\n", line)
				} else {
					fmt.Fprintf(w, "%s %s\n", prefix, line)
				}
				partial = partial[:0]
			}
		}
		if err == nil || err == bufio.ErrBufferFull {
			continue
		}

		// Flush any final partial line on EOF/other error.
		if len(partial) > 0 {
			line := bytes.TrimSuffix(partial, []byte("\r"))
			if prefix == "" {
				fmt.Fprintf(w, "%s\n", line)
			} else {
				fmt.Fprintf(w, "%s %s\n", prefix, line)
			}
		}
		break
	}
}
