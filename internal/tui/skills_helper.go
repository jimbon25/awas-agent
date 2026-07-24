package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func installSkillFromGit(repoStr string) ([]string, error) {
	repoUrl := repoStr
	if !strings.HasPrefix(repoStr, "http://") && !strings.HasPrefix(repoStr, "https://") {
		repoUrl = "https://github.com/" + repoStr
	}

	tempDir, err := os.MkdirTemp("", "awas-skill-clone-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	cmd := exec.Command("git", "clone", "--depth", "1", repoUrl, tempDir)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	skillsDir := filepath.Join(home, ".awas", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create skills directory: %w", err)
	}

	var installed []string

	err = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.ToLower(info.Name()) == "skill.md" {
			parentDir := filepath.Dir(path)
			skillName := filepath.Base(parentDir)
			if skillName == filepath.Base(tempDir) {
				parts := strings.Split(repoStr, "/")
				skillName = parts[len(parts)-1]
			}
			
			destPath := filepath.Join(skillsDir, skillName+".md")
			if err := copyFile(path, destPath); err == nil {
				installed = append(installed, skillName+".md")
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(installed) == 0 {
		readmePath := filepath.Join(tempDir, "README.md")
		if _, err := os.Stat(readmePath); err == nil {
			parts := strings.Split(repoStr, "/")
			skillName := parts[len(parts)-1]
			destPath := filepath.Join(skillsDir, skillName+".md")
			if err := copyFile(readmePath, destPath); err == nil {
				installed = append(installed, skillName+".md")
			}
		}
	}

	if len(installed) == 0 {
		return nil, fmt.Errorf("no SKILL.md or README.md found in repository")
	}

	return installed, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
