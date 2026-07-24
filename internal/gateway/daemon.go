package gateway

import (
	"awas/internal/config"
	"awas/internal/tools"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
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
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
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
