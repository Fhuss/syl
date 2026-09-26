package skill

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed assets/SKILL.md
var SkillContent string

//go:embed assets/scripts/init-syl.sh
var InitScriptContent string

type InstallTarget struct {
	Name string
	Path string
}

// GetCandidateTargets returns common agent skill directories on the host.
func GetCandidateTargets() []InstallTarget {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}

	var targets []InstallTarget

	// 1. Pi Agent
	targets = append(targets,
		InstallTarget{Name: "Pi Agent (global)", Path: filepath.Join(home, ".pi", "agent", "skills", "syl-companion")},
		InstallTarget{Name: "Pi Agent (alt)", Path: filepath.Join(home, ".pi", "skills", "syl-companion")},
	)

	// 2. Claude Code
	targets = append(targets,
		InstallTarget{Name: "Claude Code", Path: filepath.Join(home, ".claude", "skills", "syl-companion")},
	)

	// 3. Antigravity / Gemini
	targets = append(targets,
		InstallTarget{Name: "Antigravity Plugin", Path: filepath.Join(home, ".gemini", "config", "plugins", "awesome-skills-plugin", "skills", "syl-companion")},
		InstallTarget{Name: "Antigravity (local)", Path: filepath.Join(home, ".gemini", "antigravity-cli", "skills", "syl-companion")},
	)

	// 4. Central Syl Config
	targets = append(targets,
		InstallTarget{Name: "Syl Central Config", Path: filepath.Join(home, ".config", "syl", "skills", "syl-companion")},
	)

	return targets
}

// InstallSkill copies embedded skill assets into detected or requested agent skill paths.
func InstallSkill(all bool, specificTarget string) ([]string, error) {
	candidates := GetCandidateTargets()
	var installed []string

	for _, t := range candidates {
		// If a specific target was given, filter by name or path
		if specificTarget != "" {
			if t.Name != specificTarget && t.Path != specificTarget {
				continue
			}
		} else if !all {
			// By default, install to existing agent directories or Central Config
			parentDir := filepath.Dir(filepath.Dir(t.Path))
			if _, err := os.Stat(parentDir); err != nil && t.Name != "Syl Central Config" {
				continue
			}
		}

		if err := writeSkillFiles(t.Path); err != nil {
			return installed, fmt.Errorf("failed to install to %s: %w", t.Path, err)
		}
		installed = append(installed, fmt.Sprintf("%s (%s)", t.Name, t.Path))
	}

	// Always ensure Central Config has the latest scripts
	home, _ := os.UserHomeDir()
	centralScript := filepath.Join(home, ".config", "syl", "scripts", "init-syl.sh")
	_ = os.MkdirAll(filepath.Dir(centralScript), 0755)
	_ = os.WriteFile(centralScript, []byte(InitScriptContent), 0755)

	return installed, nil
}

func writeSkillFiles(destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	skillFile := filepath.Join(destDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(SkillContent), 0644); err != nil {
		return err
	}

	scriptDir := filepath.Join(destDir, "scripts")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		return err
	}

	scriptFile := filepath.Join(scriptDir, "init-syl.sh")
	if err := os.WriteFile(scriptFile, []byte(InitScriptContent), 0755); err != nil {
		return err
	}

	return nil
}

// Launch runs the embedded companion init script to split terminal and launch Syl.
func Launch(force bool, check bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}

	scriptPath := filepath.Join(home, ".config", "syl", "scripts", "init-syl.sh")
	_ = os.MkdirAll(filepath.Dir(scriptPath), 0755)
	if err := os.WriteFile(scriptPath, []byte(InitScriptContent), 0755); err != nil {
		return fmt.Errorf("write launch script: %w", err)
	}

	var args []string
	if force {
		args = append(args, "--force")
	}
	if check {
		args = append(args, "--check")
	}

	cmd := exec.Command("/usr/bin/env", append([]string{"bash", scriptPath}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}
