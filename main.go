package main

import (
	"awas/internal/config"
	"awas/internal/gateway"
	"awas/internal/tui"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var Version = "0.1.2"

func main() {
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "--version" || arg == "-v" || arg == "version" {
			fmt.Printf("awas version %s\n", Version)
			os.Exit(0)
		}
	}

	if len(os.Args) > 1 && os.Args[1] == "gateway" {
		handleGatewayCommand()
		return
	}

	cfg := config.Load()

	var initialQuery string
	if len(os.Args) > 1 {
		initialQuery = strings.Join(os.Args[1:], " ")
	}

	if err := tui.Run(cfg, initialQuery); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func handleGatewayCommand() {
	if len(os.Args) < 3 {
		printGatewayUsage()
		return
	}

	sub := os.Args[2]
	switch sub {
	case "run":
		cfg := config.Load()
		if err := cfg.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := gateway.RunDaemon(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "start":
		binaryPath, err := getBinaryPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := gateway.InstallService(binaryPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "restart":
		binaryPath, err := getBinaryPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := gateway.RestartService(binaryPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "stop":
		if err := gateway.UninstallService(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "status":
		fmt.Println(gateway.ServiceStatus())

	case "uninstall":
		if err := gateway.UninstallService(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	default:
		printGatewayUsage()
	}
}

func printGatewayUsage() {
	fmt.Println(`Usage: awas gateway <command>

Commands:
  start       Install and start AWAS Gateway as a system service
  stop        Stop and uninstall the gateway service
  restart     Stop, reinstall, and start the gateway service
  status      Show gateway service status
  run         Run gateway in foreground (used by service manager)
  uninstall   Remove the gateway service

Examples:
  awas gateway start       # Install & start as systemd/launchd service
  awas gateway restart     # Restart service with a new binary (e.g. after update)
  awas gateway stop        # Stop & remove service
  awas gateway status      # Check if running`)
	os.Exit(1)
}

func getBinaryPath() (string, error) {
	binPath, err := exec.LookPath("awas")
	if err == nil {
		return binPath, nil
	}

	binPath, err = os.Executable()
	if err == nil {
		return filepath.EvalSymlinks(binPath)
	}

	return "", fmt.Errorf("cannot find awas binary")
}
