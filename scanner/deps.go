package scanner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type DependencyStatus struct {
	Name        string
	Available   bool
	InstallHint string
	Reason      string
	AutoInstall bool
}

func userSupportDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".keyword-hunter")
	}
	return filepath.Join(home, ".keyword-hunter")
}

func msgVenvDir() string {
	return filepath.Join(userSupportDir(), "msg-support")
}

func msgPythonPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(msgVenvDir(), "Scripts", "python.exe")
	}
	return filepath.Join(msgVenvDir(), "bin", "python")
}

func msgPipPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(msgVenvDir(), "Scripts", "pip.exe")
	}
	return filepath.Join(msgVenvDir(), "bin", "pip")
}

func python3Command() (string, bool) {
	candidates := []string{"python3"}
	if runtime.GOOS == "windows" {
		candidates = []string{"python", "py", "python3"}
	}
	for _, candidate := range candidates {
		if path, ok := dependencyPath(candidate); ok {
			return path, true
		}
	}
	return "", false
}

func dependencyPath(name string) (string, bool) {
	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}
	for _, candidate := range dependencyPathCandidates(name) {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, true
		}
	}
	return "", false
}

func dependencyPathCandidates(name string) []string {
	candidates := []string{}
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates,
			"/opt/homebrew/bin/"+name,
			"/usr/local/bin/"+name,
		)
	case "linux":
		candidates = append(candidates,
			"/usr/bin/"+name,
			"/usr/local/bin/"+name,
			"/snap/bin/"+name,
		)
	}
	return candidates
}

func installHintForReadPST() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install libpst"
	case "linux":
		return "Install libpst/readpst with your package manager, for example: sudo apt install libpst-dev pst-utils"
	default:
		return "Install libpst/readpst with your system package manager"
	}
}

func installHintForPython3() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install python"
	case "linux":
		return "Install Python 3 with your package manager, for example: sudo apt install python3 python3-venv python3-pip"
	default:
		return "Install Python 3 with your system package manager"
	}
}

func installHintForExtractMSG() string {
	if runtime.GOOS == "windows" {
		return "Create a virtual environment and install extract-msg with pip"
	}
	return "python3 -m venv ~/.keyword-hunter/msg-support && ~/.keyword-hunter/msg-support/bin/pip install extract-msg"
}

func HasPython3() bool {
	_, ok := python3Command()
	return ok
}

func HasExtractMSG() bool {
	pythonPath := msgPythonPath()
	if _, err := os.Stat(pythonPath); err == nil {
		cmd := exec.Command(pythonPath, "-c", "import extract_msg")
		return cmd.Run() == nil
	}
	if !HasPython3() {
		return false
	}
	cmd := exec.Command("python3", "-c", "import extract_msg")
	return cmd.Run() == nil
}

func HasHighFidelityMSG() bool {
	return HasPython3() && HasExtractMSG()
}

func Python3DependencyStatus() DependencyStatus {
	status := DependencyStatus{
		Name:        "python3",
		Available:   HasPython3(),
		InstallHint: installHintForPython3(),
		AutoInstall: runtime.GOOS == "darwin",
	}
	if !status.Available {
		status.Reason = "python3 is not installed"
	}
	return status
}

func ExtractMSGDependencyStatus() DependencyStatus {
	status := DependencyStatus{
		Name:        "extract-msg",
		Available:   HasExtractMSG(),
		InstallHint: installHintForExtractMSG(),
		AutoInstall: runtime.GOOS == "darwin",
	}
	if !status.Available {
		if !HasPython3() {
			status.Reason = "python3 is required before extract-msg can be installed"
		} else {
			status.Reason = "extract-msg is not installed"
		}
	}
	return status
}

func MSGDependencyStatus() DependencyStatus {
	status := DependencyStatus{
		Name:        "high-fidelity MSG support",
		InstallHint: installHintForExtractMSG(),
		AutoInstall: runtime.GOOS == "darwin",
	}
	switch {
	case HasHighFidelityMSG():
		status.Available = true
	default:
		status.Reason = "python3 and extract-msg are both required"
	}
	return status
}

func ReadPSTDependencyStatus() DependencyStatus {
	status := DependencyStatus{
		Name:        "readpst",
		Available:   HasReadPST(),
		InstallHint: installHintForReadPST(),
		AutoInstall: runtime.GOOS == "darwin",
	}
	if !status.Available {
		status.Reason = "readpst is not installed"
	}
	return status
}

func InstallReadPST() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("automatic install is only available on macOS; run: %s", installHintForReadPST())
	}
	if _, ok := dependencyPath("brew"); !ok {
		return fmt.Errorf("Homebrew is not installed; run: %s", installHintForReadPST())
	}
	cmd := exec.Command("brew", "install", "libpst")
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func InstallPython3() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("automatic install is only available on macOS; run: %s", installHintForPython3())
	}
	if _, ok := dependencyPath("brew"); !ok {
		return fmt.Errorf("Homebrew is not installed; run: %s", installHintForPython3())
	}
	cmd := exec.Command("brew", "install", "python")
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func InstallHighFidelityMSG() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("automatic install is only available on macOS; run: %s", installHintForExtractMSG())
	}
	if !HasPython3() {
		return fmt.Errorf("python3 is not installed; install python3 before enabling MSG support")
	}
	if err := os.MkdirAll(msgVenvDir(), 0755); err != nil {
		return err
	}
	pythonCmd, ok := python3Command()
	if !ok {
		return fmt.Errorf("python3 is not installed; install python3 before enabling MSG support")
	}
	venvArgs := []string{"-m", "venv", msgVenvDir()}
	if pythonCmd == "py" {
		venvArgs = append([]string{"-3"}, venvArgs...)
	}
	create := exec.Command(pythonCmd, venvArgs...)
	if output, err := create.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	pipPath := msgPipPath()
	install := exec.Command(pipPath, "install", "extract-msg")
	output, err := install.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}
