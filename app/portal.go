package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/godbus/dbus/v5"
)

// The file dialogs are xdg-desktop-portal's, the ones every other app on
// the desktop shows, asked for over D-Bus.
const (
	portalDest        = "org.freedesktop.portal.Desktop"
	portalPath        = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	portalFileChooser = "org.freedesktop.portal.FileChooser"
	portalRequest     = "org.freedesktop.portal.Request"
)

// portalFilter is one entry of the dialog's file type list, D-Bus type
// (sa(us)).
type portalFilter struct {
	Name  string
	Rules []portalRule
}

// portalRule is a glob (kind 0) or MIME type (kind 1).
type portalRule struct {
	Kind    uint32
	Pattern string
}

var portalFilters = []portalFilter{
	{"Markdown and Mermaid", []portalRule{
		{0, "*.md"}, {0, "*.markdown"}, {0, "*.mmd"}, {0, "*.mermaid"},
	}},
	{"All files", []portalRule{{0, "*"}}},
}

// PortalOpen shows the desktop's open dialog and returns the file picked,
// or "" if the dialog was cancelled. It blocks until the dialog closes.
func PortalOpen(title string) (string, error) {
	return portalChoose("OpenFile", title, map[string]dbus.Variant{
		"filters": dbus.MakeVariant(portalFilters),
	})
}

// PortalSave shows the desktop's save dialog, starting at current when
// the document already has a file, and returns the path picked, or "" if
// the dialog was cancelled. It blocks until the dialog closes.
func PortalSave(title, current string) (string, error) {
	opts := map[string]dbus.Variant{
		"filters":      dbus.MakeVariant(portalFilters),
		"current_name": dbus.MakeVariant("Untitled.md"),
	}
	if current != "" {
		opts["current_name"] = dbus.MakeVariant(filepath.Base(current))
		// Paths go over as NUL-terminated bytes.
		opts["current_folder"] = dbus.MakeVariant(append([]byte(filepath.Dir(current)), 0))
	}
	return portalChoose("SaveFile", title, opts)
}

// portalChoose calls the FileChooser method and waits for the Response
// signal on the request it starts.
func portalChoose(method, title string, opts map[string]dbus.Variant) (string, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return "", fmt.Errorf("file dialog: %w", err)
	}
	defer conn.Close()

	// The request's path is made from our bus name and a token we choose,
	// so its signal can be listened for before the call: a dialog that
	// closes at once could otherwise answer before we listen.
	tok := make([]byte, 8)
	rand.Read(tok)
	token := "dyamark_" + hex.EncodeToString(tok)
	opts["handle_token"] = dbus.MakeVariant(token)
	sender := strings.ReplaceAll(strings.TrimPrefix(conn.Names()[0], ":"), ".", "_")
	request := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + sender + "/" + token)

	signals := make(chan *dbus.Signal, 4)
	conn.Signal(signals)
	listen := func(path dbus.ObjectPath) error {
		return conn.AddMatchSignal(
			dbus.WithMatchObjectPath(path),
			dbus.WithMatchInterface(portalRequest),
			dbus.WithMatchMember("Response"),
		)
	}
	if err := listen(request); err != nil {
		return "", fmt.Errorf("file dialog: %w", err)
	}

	var handle dbus.ObjectPath
	call := conn.Object(portalDest, portalPath).Call(portalFileChooser+"."+method, 0, "", title, opts)
	if err := call.Store(&handle); err != nil {
		return "", fmt.Errorf("file dialog: %w", err)
	}
	// Portals older than version 0.9 pick their own path.
	if handle != request {
		if err := listen(handle); err != nil {
			return "", fmt.Errorf("file dialog: %w", err)
		}
	}

	for sig := range signals {
		if sig.Name != portalRequest+".Response" || (sig.Path != request && sig.Path != handle) {
			continue
		}
		return portalResponse(sig.Body)
	}
	return "", fmt.Errorf("file dialog: lost the connection to the session bus")
}

// portalResponse reads a Response signal's body, (u response, a{sv}
// results): 0 is a choice, 1 a cancel, anything else a failure.
func portalResponse(body []any) (string, error) {
	if len(body) != 2 {
		return "", fmt.Errorf("file dialog: unexpected response %v", body)
	}
	code, _ := body[0].(uint32)
	switch code {
	case 0:
	case 1:
		return "", nil
	default:
		return "", fmt.Errorf("file dialog failed")
	}
	results, _ := body[1].(map[string]dbus.Variant)
	uris, _ := results["uris"].Value().([]string)
	if len(uris) == 0 {
		return "", fmt.Errorf("file dialog chose no file")
	}
	return uriPath(uris[0])
}

// uriPath turns a file:// URI into a path.
func uriPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("%s isn't a local file", uri)
	}
	return u.Path, nil
}
