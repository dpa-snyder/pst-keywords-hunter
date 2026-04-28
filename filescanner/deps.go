package filescanner

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func installHintForPDFToText() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install poppler"
	case "linux":
		return "Install poppler-utils with your package manager, for example: sudo apt install poppler-utils"
	default:
		return "Install pdftotext/poppler with your system package manager"
	}
}

func installHintForSoffice() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install --cask libreoffice"
	case "linux":
		return "Install LibreOffice with your package manager, for example: sudo apt install libreoffice"
	default:
		return "Install LibreOffice with your system package manager"
	}
}

func HasPDFToText() bool {
	_, ok := dependencyPath("pdftotext")
	return ok
}

func HasSoffice() bool {
	_, ok := dependencyPath("soffice")
	return ok
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
		if name == "soffice" {
			candidates = append(candidates, "/Applications/LibreOffice.app/Contents/MacOS/soffice")
		}
	case "linux":
		candidates = append(candidates,
			"/usr/bin/"+name,
			"/usr/local/bin/"+name,
			"/snap/bin/"+name,
		)
	}
	return candidates
}

func DependencyStatuses() []DependencyStatus {
	pdf := DependencyStatus{
		Key:         "pdftotext",
		Name:        "pdftotext / poppler",
		Available:   HasPDFToText(),
		InstallHint: installHintForPDFToText(),
		AutoInstall: runtime.GOOS == "darwin",
	}
	if !pdf.Available {
		pdf.Reason = "PDF content search will fall back to filename-only."
	}

	soffice := DependencyStatus{
		Key:         "soffice",
		Name:        "LibreOffice / soffice",
		Available:   HasSoffice(),
		InstallHint: installHintForSoffice(),
		AutoInstall: runtime.GOOS == "darwin",
	}
	if !soffice.Available {
		soffice.Reason = "Legacy Office and OpenDocument content search will fall back to filename-only."
	}

	return []DependencyStatus{pdf, soffice}
}

func InstallDependency(key string) error {
	if runtime.GOOS != "darwin" {
		switch key {
		case "pdftotext":
			return fmt.Errorf("automatic install is only available on macOS; run: %s", installHintForPDFToText())
		case "soffice":
			return fmt.Errorf("automatic install is only available on macOS; run: %s", installHintForSoffice())
		default:
			return fmt.Errorf("unknown dependency: %s", key)
		}
	}
	if _, err := exec.LookPath("brew"); err != nil {
		return fmt.Errorf("Homebrew is not installed")
	}

	var cmd *exec.Cmd
	switch key {
	case "pdftotext":
		cmd = exec.Command("brew", "install", "poppler")
	case "soffice":
		cmd = exec.Command("brew", "install", "--cask", "libreoffice")
	default:
		return fmt.Errorf("unknown dependency: %s", key)
	}

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
