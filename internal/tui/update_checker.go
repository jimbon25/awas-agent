package tui

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type UpdateCheckMsg struct {
	LatestVersion string
	Err           error
}

type AutoUpdateMsg struct {
	Status     string 
	NewVersion string
	Err        error
	Output     string
}

type npmLatestResponse struct {
	Version string `json:"version"`
}

func CheckForUpdates(currentVersion string) tea.Cmd {
	return func() tea.Msg {
		client := http.Client{
			Timeout: 2 * time.Second,
		}

		resp, err := client.Get("https://registry.npmjs.org/awas-agent/latest")
		if err != nil {
			return UpdateCheckMsg{Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return UpdateCheckMsg{}
		}

		var data npmLatestResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return UpdateCheckMsg{Err: err}
		}

		latest := strings.TrimPrefix(strings.TrimSpace(data.Version), "v")
		curr := strings.TrimPrefix(strings.TrimSpace(currentVersion), "v")

		if latest != "" && isNewerVersion(curr, latest) {
			return UpdateCheckMsg{LatestVersion: latest}
		}

		return UpdateCheckMsg{}
	}
}

func AutoUpdate(latestVersion string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("npm", "i", "-g", "awas-agent@"+latestVersion)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return AutoUpdateMsg{Status: "error", Err: err, Output: string(output)}
		}
		return AutoUpdateMsg{Status: "done", NewVersion: latestVersion}
	}
}

func isNewerVersion(current, latest string) bool {
	if current == latest {
		return false
	}
	cParts := strings.Split(current, ".")
	lParts := strings.Split(latest, ".")

	for i := 0; i < len(cParts) && i < len(lParts); i++ {
		cNum, err1 := strconv.Atoi(cParts[i])
		lNum, err2 := strconv.Atoi(lParts[i])
		if err1 == nil && err2 == nil {
			if lNum > cNum {
				return true
			} else if lNum < cNum {
				return false
			}
		} else {
			if lParts[i] > cParts[i] {
				return true
			} else if lParts[i] < cParts[i] {
				return false
			}
		}
	}
	return len(lParts) > len(cParts)
}
