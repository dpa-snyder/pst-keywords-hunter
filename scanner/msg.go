package scanner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const extractMsgScript = `
import os
import sys
from email.message import EmailMessage
from email.utils import format_datetime

import extract_msg

msg_path, out_path = sys.argv[1], sys.argv[2]
message = extract_msg.openMsg(msg_path)
email_message = EmailMessage()

sender = getattr(message, "sender", None)
to = getattr(message, "to", None)
cc = getattr(message, "cc", None)
bcc = getattr(message, "bcc", None)
subject = getattr(message, "subject", None) or os.path.basename(msg_path)
date_value = getattr(message, "date", None)
body = getattr(message, "body", None) or ""

if sender:
    email_message["From"] = str(sender)
if to:
    email_message["To"] = str(to)
if cc:
    email_message["Cc"] = str(cc)
if bcc:
    email_message["Bcc"] = str(bcc)
email_message["Subject"] = str(subject)
if date_value:
    if hasattr(date_value, "tzinfo"):
        email_message["Date"] = format_datetime(date_value)
    else:
        email_message["Date"] = str(date_value)
email_message["MIME-Version"] = "1.0"
email_message["X-Converted-From"] = "MSG"
email_message["X-Original-File"] = os.path.basename(msg_path)
email_message.set_content(str(body))

with open(out_path, "wb") as handle:
    handle.write(email_message.as_bytes())

try:
    message.close()
except Exception:
    pass
`

// ConvertMSGToEML converts a .msg file to .eml using extract-msg.
func ConvertMSGToEML(msgPath string, outDir string) (string, error) {
	python, err := msgPythonInterpreter()
	if err != nil {
		return "", err
	}

	emlPath := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(msgPath), filepath.Ext(msgPath))+".eml")
	cmd := exec.Command(python, "-c", extractMsgScript, msgPath, emlPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("high-fidelity MSG conversion failed: %s", msg)
	}
	if _, err := os.Stat(emlPath); err != nil {
		return "", fmt.Errorf("high-fidelity MSG conversion did not produce an EML file")
	}
	return emlPath, nil
}

func msgPythonInterpreter() (string, error) {
	if python := msgPythonPath(); python != "" {
		if _, err := os.Stat(python); err == nil {
			return python, nil
		}
	}
	if HasHighFidelityMSG() {
		return "python3", nil
	}
	return "", fmt.Errorf("high-fidelity MSG support is unavailable because extract-msg is not installed")
}
