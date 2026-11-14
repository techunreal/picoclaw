package browser

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ErrInstallRunning is returned when a browser download is already in progress.
var ErrInstallRunning = errors.New("browser engine download is already running")

// InstallStatus describes the background browser engine download.
type InstallStatus struct {
	Running    bool   `json:"running"`
	Done       bool   `json:"done"`
	Error      string `json:"error,omitempty"`
	LastLine   string `json:"last_line,omitempty"`
	StartedAt  int64  `json:"started_at,omitempty"`
	FinishedAt int64  `json:"finished_at,omitempty"`
}

var (
	installMu     sync.Mutex
	installState  InstallStatus
	installCancel context.CancelFunc
)

// StartBrowserInstall runs "agent-browser install" in the background. The
// download is large (hundreds of MB) and only starts when the operator asks.
func StartBrowserInstall(binaryPath string) error {
	binary, err := resolveBinary(binaryPath)
	if err != nil {
		return err
	}

	installMu.Lock()
	if installState.Running {
		installMu.Unlock()
		return ErrInstallRunning
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	installCancel = cancel
	installState = InstallStatus{
		Running:   true,
		StartedAt: time.Now().UnixMilli(),
	}
	installMu.Unlock()

	go func() {
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "install")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			finishInstall(err)
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			finishInstall(err)
			return
		}
		if err := cmd.Start(); err != nil {
			finishInstall(err)
			return
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go scanInstallLines(stdout, &wg)
		go scanInstallLines(stderr, &wg)
		wg.Wait()

		waitErr := cmd.Wait()
		if ctx.Err() != nil {
			waitErr = fmt.Errorf("browser download timed out")
		}
		finishInstall(waitErr)
	}()

	return nil
}

// BrowserInstallStatus returns a snapshot of the download state.
func BrowserInstallStatus() InstallStatus {
	installMu.Lock()
	defer installMu.Unlock()
	return installState
}

// CancelBrowserInstall stops a running download.
func CancelBrowserInstall() {
	installMu.Lock()
	defer installMu.Unlock()
	if installCancel != nil {
		installCancel()
	}
}

func scanInstallLines(reader interface{ Read([]byte) (int, error) }, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(reader)
	scanner.Split(scanLinesAndCarriageReturns)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 256*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		installMu.Lock()
		installState.LastLine = line
		installMu.Unlock()
	}
}

func scanLinesAndCarriageReturns(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := strings.IndexAny(string(data), "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func finishInstall(err error) {
	installMu.Lock()
	defer installMu.Unlock()
	installState.Running = false
	installState.Done = true
	installState.FinishedAt = time.Now().UnixMilli()
	if err != nil {
		installState.Error = err.Error()
	}
	installCancel = nil
}
