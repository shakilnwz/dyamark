package app

import (
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// launch opens target, a URL or a file, in the desktop's app for it. Tests
// replace it.
var launch = func(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// openLink follows a clicked link. A Markdown or Mermaid file opens here,
// found next to the open file when the link is relative; anything else
// opens in the desktop's app for it. Links within the page do nothing yet.
func (a *App) openLink(link string) {
	u, err := url.Parse(link)
	if err != nil || link == "" || strings.HasPrefix(link, "#") {
		return
	}
	target := link
	if u.Scheme == "" || u.Scheme == "file" {
		target = u.Path
		if !filepath.IsAbs(target) && a.opts.FilePath != "" {
			target = filepath.Join(filepath.Dir(a.opts.FilePath), target)
		}
		switch strings.ToLower(filepath.Ext(target)) {
		case ".md", ".markdown", ".mmd":
			a.whenSaved(func() { a.open(target) })
			return
		}
		if !filepath.IsAbs(target) {
			// No file to say where it is from.
			return
		}
	}
	if err := launch(target); err != nil {
		a.notify("Couldn't open " + link + ": " + err.Error())
	}
}
