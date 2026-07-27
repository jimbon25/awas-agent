//go:build linux

package gateway

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const systemdUnit = `[Unit]
Description=AWAS Gateway Daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart={{.BinaryPath}} gateway run
Restart=on-failure
RestartSec=5
Environment=AWAS_WORKDIR={{.WorkDir}}

[Install]
WantedBy=default.target
`

type unitData struct {
	BinaryPath string
	WorkDir    string
}

func GetSystemdUnitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "awas-gateway.service")
}

func InstallService(binaryPath string) error {
	unitPath := GetSystemdUnitPath()

	dir := filepath.Dir(unitPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create systemd directory: %v", err)
	}

	home, _ := os.UserHomeDir()
	workDir := home

	tmpl, err := template.New("unit").Parse(systemdUnit)
	if err != nil {
		return err
	}

	f, err := os.Create(unitPath)
	if err != nil {
		return fmt.Errorf("failed to create unit file: %v", err)
	}
	defer f.Close()

	data := unitData{
		BinaryPath: binaryPath,
		WorkDir:    workDir,
	}

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to write unit file: %v", err)
	}

	if err := runCmd("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %v", err)
	}

	if err := runCmd("systemctl", "--user", "enable", "awas-gateway.service"); err != nil {
		return fmt.Errorf("failed to enable service: %v", err)
	}

	if err := runCmd("systemctl", "--user", "start", "awas-gateway.service"); err != nil {
		return fmt.Errorf("failed to start service: %v", err)
	}

	fmt.Println("✔ AWAS Gateway service installed and started.")
	fmt.Printf("   Unit file: %s\n", unitPath)
	fmt.Println("   Manage with: systemctl --user [start|stop|status|restart] awas-gateway")
	return nil
}

func UninstallService() error {
	runCmd("systemctl", "--user", "stop", "awas-gateway.service")

	runCmd("systemctl", "--user", "disable", "awas-gateway.service")

	unitPath := GetSystemdUnitPath()
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unit file: %v", err)
	}

	runCmd("systemctl", "--user", "daemon-reload")

	fmt.Println("✔ AWAS Gateway service uninstalled.")
	return nil
}

func ServiceStatus() string {
	out, err := exec.Command("systemctl", "--user", "status", "awas-gateway.service").CombinedOutput()
	if err != nil {
		return "Service not installed or not running"
	}
	return string(out)
}

func RestartService(binaryPath string) error {
	_ = UninstallService()
	if err := InstallService(binaryPath); err != nil {
		return err
	}
	fmt.Println("✔ AWAS Gateway service restarted.")
	return nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
