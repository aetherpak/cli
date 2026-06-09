package executil

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
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

// wrapCommand wraps command execution in a transient D-Bus session if no D-Bus session is active
// and dbus-run-session is available in the PATH.
func wrapCommand(lookPath func(string) (string, error), getenv func(string) string, name string, args ...string) (string, []string) {
	if (name == "flatpak" || name == "flatpak-builder") && getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		if _, err := lookPath("dbus-run-session"); err == nil {
			newArgs := append([]string{"--", name}, args...)
			return "dbus-run-session", newArgs
		}
	}
	return name, args
}

// Command creates a new Command executing the given command and arguments on the OS.
func (e *OSExecutor) Command(name string, arg ...string) Command {
	cmdName, cmdArgs := wrapCommand(exec.LookPath, os.Getenv, name, arg...)
	return &osCommand{cmd: exec.Command(cmdName, cmdArgs...)}
}

// LookPath searches for an executable binary in the system PATH.
func (e *OSExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

type osCommand struct {
	cmd *exec.Cmd
}

func (c *osCommand) Run() error {
	return c.cmd.Run()
}

func (c *osCommand) Start() error {
	return c.cmd.Start()
}

func (c *osCommand) Wait() error {
	return c.cmd.Wait()
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
