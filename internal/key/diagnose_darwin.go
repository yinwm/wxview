//go:build darwin

package key

import (
	"context"
	"os/exec"
	"strings"
)

const wechatAppPath = "/Applications/WeChat.app"

func wechatPermissionHint(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "codesign", "-dv", wechatAppPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return " Try sudo, run from a local GUI terminal with Developer Tools permission, or re-sign WeChat ad-hoc and restart it."
	}
	return buildPermissionHint(string(out))
}

func buildPermissionHint(codesignOutput string) string {
	lines := compactCodesignLines(codesignOutput)
	base := " WeChat signature: " + strings.Join(lines, "; ") + "."
	if strings.Contains(codesignOutput, "runtime") {
		return base + " This is a Hardened Runtime build; sudo alone can still fail with task_for_pid=5. Grant your terminal app Developer Tools permission, or quit WeChat, run `sudo xattr -cr /Applications/WeChat.app` and `sudo codesign --force --deep --sign - /Applications/WeChat.app`, then restart WeChat."
	}
	if strings.Contains(codesignOutput, "Signature=adhoc") || strings.Contains(codesignOutput, "flags=0x2(adhoc)") {
		return base + " This looks ad-hoc signed; sudo should usually work. Make sure WeChat was restarted after signing and run from the local GUI user session."
	}
	return base + " Try sudo, run from a local GUI terminal with Developer Tools permission, or re-sign WeChat ad-hoc and restart it."
}

func compactCodesignLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "flags=") ||
			strings.HasPrefix(line, "Signature=") ||
			strings.HasPrefix(line, "TeamIdentifier=") ||
			strings.HasPrefix(line, "Identifier=") {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return []string{"unknown"}
	}
	return lines
}
