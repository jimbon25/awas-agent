//go:build darwin

package gateway

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const launchdPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.awas.gateway</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>gateway</string>
        <string>run</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{.LogPath}}</string>
    <key>StandardErrorPath</key>
    <string>{{.LogPath}}</string>
    <key>WorkingDirectory</key>
    <string>{{.WorkDir}}</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>AWAS_WORKDIR</key>
        <string>{{.WorkDir}}</string>
    </dict>
</dict>
</plist>
`

type plistData struct {
	BinaryPath string
	WorkDir    string
	LogPath    string
}

func getPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.awas.gateway.plist")
}

func InstallService(binaryPath string) error {
	plistPath := getPlistPath()

	dir := filepath.Dir(plistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %v", err)
	}

	home, _ := os.UserHomeDir()
	workDir := home
	logPath := filepath.Join(home, ".awas", "gateway.log")

	tmpl, err := template.New("plist").Parse(launchdPlist)
	if err != nil {
		return err
	}

	f, err := os.Create(plistPath)
	if err != nil {
		return fmt.Errorf("failed to create plist file: %v", err)
	}
	defer f.Close()

	data := plistData{
		BinaryPath: binaryPath,
		WorkDir:    workDir,
		LogPath:    logPath,
	}

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to write plist: %v", err)
	}

	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("failed to load launchd service: %v", err)
	}

	fmt.Println("✔ AWAS Gateway service installed and started.")
	fmt.Printf("   Plist: %s\n", plistPath)
	fmt.Println("   Manage with: launchctl [start|stop|unload] com.awas.gateway")
	return nil
}

func UninstallService() error {
	plistPath := getPlistPath()

	exec.Command("launchctl", "unload", plistPath).Run()
	os.Remove(plistPath)

	fmt.Println("✔ AWAS Gateway service uninstalled.")
	return nil
}

func ServiceStatus() string {
	out, err := exec.Command("launchctl", "list", "com.awas.gateway").CombinedOutput()
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
