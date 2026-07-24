package tools

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func SystemEnv() string {
	var sb strings.Builder
	sb.WriteString("🖥️ **System Environment Information**\n\n")

	// OS Info
	sb.WriteString("### OS & Architecture\n")
	sb.WriteString(fmt.Sprintf("- **OS**: %s\n", runtime.GOOS))
	sb.WriteString(fmt.Sprintf("- **Arch**: %s\n", runtime.GOARCH))
	sb.WriteString(fmt.Sprintf("- **CPUs**: %d\n", runtime.NumCPU()))

	hostname, err := os.Hostname()
	if err == nil {
		sb.WriteString(fmt.Sprintf("- **Hostname**: %s\n", hostname))
	}
	sb.WriteString("\n")

	sb.WriteString("### Available Compilers & Runtimes\n")
	toolsToCheck := []string{"go", "node", "npm", "python3", "python", "pip", "gcc", "make", "docker", "git"}
	for _, tool := range toolsToCheck {
		path, err := exec.LookPath(tool)
		if err == nil {
			var version string
			var cmd *exec.Cmd
			if tool == "go" {
				cmd = exec.Command("go", "version")
			} else if tool == "node" {
				cmd = exec.Command("node", "--version")
			} else if tool == "python3" || tool == "python" {
				cmd = exec.Command(tool, "--version")
			} else if tool == "docker" {
				cmd = exec.Command("docker", "--version")
			} else {
				cmd = exec.Command(tool, "-v")
			}

			out, err := cmd.Output()
			if err == nil {
				version = strings.TrimSpace(string(out))
				if len(version) > 60 {
					version = version[:60] + "..."
				}
			} else {
				version = "Installed (version command failed)"
			}
			sb.WriteString(fmt.Sprintf("- **%s**: `%s` (Path: `%s`)\n", tool, version, path))
		} else {
			sb.WriteString(fmt.Sprintf("- **%s**: Not found\n", tool))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("### Active Local Services (Listening Ports)\n")
	commonPorts := []int{22, 80, 443, 3000, 3306, 5000, 5432, 6379, 8000, 8080, 9000, 27017}
	activePorts := []int{}

	for _, port := range commonPorts {
		address := fmt.Sprintf("127.0.0.1:%d", port)
		conn, err := net.DialTimeout("tcp", address, 10*time.Millisecond)
		if err == nil {
			activePorts = append(activePorts, port)
			conn.Close()
		}
	}

	if len(activePorts) == 0 {
		sb.WriteString("No active services detected on common ports (22, 80, 3000, 5000, 8080, etc.).\n")
	} else {
		sb.WriteString("Detected listening services on ports:\n")
		for _, port := range activePorts {
			sb.WriteString(fmt.Sprintf("- Port **%d** (Active)\n", port))
		}
	}

	return sb.String()
}
