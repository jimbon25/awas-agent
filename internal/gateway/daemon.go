package gateway

import (
	"awas/internal/config"
	"awas/internal/tools"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

const pidFile = "gateway.pid"

func PIDFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".awas", pidFile)
}

func WritePID() error {
	return os.WriteFile(PIDFilePath(), []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

func ReadPID() (int, error) {
	data, err := os.ReadFile(PIDFilePath())
	if err != nil {
		return 0, err
	}
	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	return pid, err
}

func RemovePID() {
	os.Remove(PIDFilePath())
}

func IsRunning() bool {
	pid, err := ReadPID()
	if err != nil || pid == 0 {
		return false
	}
	if !isAwasProcess(pid) {
		RemovePID()
		return false
	}
	return true
}

func isAwasProcess(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false
	}

	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
		cmdStr := string(data)
		return strings.Contains(cmdStr, "awas")
	}

	out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "comm=").Output()
	if err == nil && len(out) > 0 {
		return strings.Contains(strings.ToLower(string(out)), "awas")
	}

	outWin, errWin := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
	if errWin == nil && len(outWin) > 0 {
		return strings.Contains(strings.ToLower(string(outWin)), "awas")
	}

	return true
}

const tuiLockFile = "tui_gateway.lock"

func TUILockFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".awas", tuiLockFile)
}

func TryAcquireTUIGatewayLock() bool {
	if IsRunning() {
		return false
	}
	tuiPath := TUILockFilePath()
	data, err := os.ReadFile(tuiPath)
	if err == nil {
		var pid int
		if _, err := fmt.Sscanf(string(data), "%d", &pid); err == nil && pid > 0 {
			if process, err := os.FindProcess(pid); err == nil {
				if err := process.Signal(syscall.Signal(0)); err == nil {
					return false
				}
			}
		}
	}
	_ = os.WriteFile(tuiPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	return true
}

func ReleaseTUIGatewayLock() {
	tuiPath := TUILockFilePath()
	data, err := os.ReadFile(tuiPath)
	if err == nil {
		var pid int
		if _, err := fmt.Sscanf(string(data), "%d", &pid); err == nil && pid == os.Getpid() {
			os.Remove(tuiPath)
		}
	}
}

func RunDaemon(cfg *config.Config) error {
	if IsRunning() {
		pid, _ := ReadPID()
		return fmt.Errorf("gateway daemon already running (PID %d)", pid)
	}

	if err := WritePID(); err != nil {
		return fmt.Errorf("failed to write PID file: %v", err)
	}
	defer RemovePID()

	log.SetPrefix("[awas-gateway] ")
	log.Println("Starting gateway daemon...")

	tools.SetSearchConfig(tools.WebSearchConfig{
		SearXNGURL: cfg.SearXNGURL,
	})

	mgr := NewManager(cfg)
	if mgr.CronScheduler != nil {
		mgr.CronScheduler.Start()
		defer mgr.CronScheduler.Stop()
	}
	gwCfg := mgr.Load()

	if !gwCfg.Enabled {
		return fmt.Errorf("no gateways enabled in ~/.awas/gateways.json")
	}

	started := 0
	for name, platform := range gwCfg.Platforms {
		if platform.Enabled {
			log.Printf("Starting %s gateway...", name)
			if err := mgr.Start(name); err != nil {
				log.Printf("Warning: failed to start %s: %v", name, err)
				continue
			}
			started++
		}
	}

	if started == 0 {
		return fmt.Errorf("no gateways could be started")
	}

	log.Printf("All gateways started (%d active). Waiting for signals...", started)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("Received signal %v, shutting down...", sig)

	mgr.StopAll()
	log.Println("Gateway daemon stopped.")
	return nil
}

func StopDaemon() error {
	pid, err := ReadPID()
	if err != nil || pid == 0 {
		return fmt.Errorf("no gateway daemon running")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %v", pid, err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop daemon (PID %d): %v", pid, err)
	}

	RemovePID()
	fmt.Printf("Gateway daemon (PID %d) stopped.\n", pid)
	return nil
}
