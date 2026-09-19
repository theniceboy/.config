package main

import (
	"fmt"
	"os/exec"
	"strings"
)

const defaultClipboardSyncHost = "dmini"

func clipboardSyncSSHArgs(host string) []string {
	return []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		host,
	}
}

func runClipboardSync(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent clip <pull|push> [--host <ssh-host>]")
	}
	direction := args[0]
	host := defaultClipboardSyncHost
	for i := 1; i < len(args); i++ {
		if args[i] == "--host" && i+1 < len(args) {
			host = args[i+1]
			i++
			continue
		}
		return fmt.Errorf("unknown flag: %s", args[i])
	}
	switch direction {
	case "pull":
		summary, err := clipboardSyncPull(host)
		if err != nil {
			return err
		}
		fmt.Println(summary)
		return nil
	case "push":
		summary, err := clipboardSyncPush(host)
		if err != nil {
			return err
		}
		fmt.Println(summary)
		return nil
	default:
		return fmt.Errorf("unknown direction %q (want pull or push)", direction)
	}
}

func clipboardSyncPull(host string) (string, error) {
	cmd := exec.Command("ssh", append(clipboardSyncSSHArgs(host), "pbpaste")...)
	remote, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ssh %s pbpaste: %w%s", host, err, stderrHint(err))
	}
	if len(strings.TrimSpace(string(remote))) == 0 {
		return "", fmt.Errorf("%s clipboard is empty; local clipboard left untouched", host)
	}
	if err := pipeToPbcopy(remote); err != nil {
		return "", err
	}
	return fmt.Sprintf("pulled %s from %s", clipboardSizeLabel(len(remote)), host), nil
}

func clipboardSyncPush(host string) (string, error) {
	local, err := exec.Command("pbpaste").Output()
	if err != nil {
		return "", fmt.Errorf("pbpaste: %w", err)
	}
	if len(strings.TrimSpace(string(local))) == 0 {
		return "", fmt.Errorf("local clipboard is empty; %s clipboard left untouched", host)
	}
	cmd := exec.Command("ssh", append(clipboardSyncSSHArgs(host), "pbcopy")...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("ssh %s pbcopy: %w", host, err)
	}
	if _, err := stdin.Write(local); err != nil {
		return "", err
	}
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("ssh %s pbcopy: %w%s", host, err, stderrHint(err))
	}
	return fmt.Sprintf("pushed %s to %s", clipboardSizeLabel(len(local)), host), nil
}

func pipeToPbcopy(data []byte) error {
	clip := exec.Command("pbcopy")
	clip.Stdin = strings.NewReader(string(data))
	return clip.Run()
}

func clipboardSizeLabel(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d bytes", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}

func stderrHint(err error) string {
	var exitErr *exec.ExitError
	if e, ok := err.(*exec.ExitError); ok {
		exitErr = e
	}
	if exitErr == nil {
		return ""
	}
	msg := strings.TrimSpace(string(exitErr.Stderr))
	if msg == "" {
		return ""
	}
	return " — " + msg
}
