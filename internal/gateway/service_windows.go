//go:build windows

package gateway

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const taskXML = `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>AWAS Gateway Daemon</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
  </Settings>
  <Actions>
    <Exec>
      <Command>{{.BinaryPath}}</Command>
      <Arguments>gateway run</Arguments>
      <WorkingDirectory>{{.WorkDir}}</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`

type taskData struct {
	BinaryPath string
	WorkDir    string
}

func getTaskXMLPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".awas", "awas-gateway.xml")
}

func InstallService(binaryPath string) error {
	xmlPath := getTaskXMLPath()

	dir := filepath.Dir(xmlPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create .awas directory: %v", err)
	}

	home, _ := os.UserHomeDir()
	workDir := home

	tmpl, err := template.New("task").Parse(taskXML)
	if err != nil {
		return err
	}

	f, err := os.Create(xmlPath)
	if err != nil {
		return fmt.Errorf("failed to create task XML: %v", err)
	}
	defer f.Close()

	data := taskData{
		BinaryPath: binaryPath,
		WorkDir:    workDir,
	}

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to write task XML: %v", err)
	}

	if err := exec.Command("schtasks", "/create", "/tn", "AWAS Gateway", "/xml", xmlPath, "/f").Run(); err != nil {
		return fmt.Errorf("failed to create scheduled task: %v", err)
	}

	if err := exec.Command("schtasks", "/run", "/tn", "AWAS Gateway").Run(); err != nil {
		return fmt.Errorf("failed to start scheduled task: %v", err)
	}

	fmt.Println("✔ AWAS Gateway service installed and started.")
	fmt.Printf("   Task XML: %s\n", xmlPath)
	fmt.Println("   Manage with: schtasks [/run|/end|/delete] /tn \"AWAS Gateway\"")
	return nil
}

func UninstallService() error {
	exec.Command("schtasks", "/end", "/tn", "AWAS Gateway").Run()
	exec.Command("schtasks", "/delete", "/tn", "AWAS Gateway", "/f").Run()
	os.Remove(getTaskXMLPath())

	fmt.Println("✔ AWAS Gateway service uninstalled.")
	return nil
}

func ServiceStatus() string {
	out, err := exec.Command("schtasks", "/query", "/tn", "AWAS Gateway", "/fo", "LIST").CombinedOutput()
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
