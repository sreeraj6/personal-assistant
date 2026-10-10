// Package platform wraps desktop process launching.
package platform

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
)

func Open(ctx context.Context, path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", path)
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", path)
	default:
		if filepath.Ext(path) == ".xlsx" {
			office, err := exec.LookPath("libreoffice")
			if err != nil {
				return errors.New("install LibreOffice to open the workbook in a native Linux app")
			}
			cmd = exec.CommandContext(ctx, office, "--calc", path)
		} else {
			cmd = exec.CommandContext(ctx, "xdg-open", path)
		}
	}
	return cmd.Run()
}
