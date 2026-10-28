package app

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/godbus/dbus/v5"
)

// fakePortal answers FileChooser calls like xdg-desktop-portal: it returns
// the request path at once and answers on it with a Response signal.
type fakePortal struct {
	conn *dbus.Conn
	got  chan map[string]dbus.Variant

	mu       sync.Mutex
	response uint32
	uri      string
}

// set changes the answer the next dialog gives.
func (p *fakePortal) set(response uint32, uri string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.response, p.uri = response, uri
}

func (p *fakePortal) answer(sender dbus.Sender, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	p.got <- opts
	token, _ := opts["handle_token"].Value().(string)
	name := strings.ReplaceAll(strings.TrimPrefix(string(sender), ":"), ".", "_")
	path := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + name + "/" + token)
	p.mu.Lock()
	response, uri := p.response, p.uri
	p.mu.Unlock()
	go p.conn.Emit(path, portalRequest+".Response", response, map[string]dbus.Variant{
		"uris": dbus.MakeVariant([]string{uri}),
	})
	return path, nil
}

func (p *fakePortal) OpenFile(sender dbus.Sender, parent, title string, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	return p.answer(sender, opts)
}

func (p *fakePortal) SaveFile(sender dbus.Sender, parent, title string, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	return p.answer(sender, opts)
}

// startFakePortal runs a private session bus with a fake portal on it.
func startFakePortal(t *testing.T) *fakePortal {
	t.Helper()
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("no dbus-daemon")
	}
	// The bus has no service directories, so a missing portal isn't
	// started from the system's service files.
	dir := t.TempDir()
	config := filepath.Join(dir, "bus.conf")
	if err := os.WriteFile(config, []byte(`<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:path=`+filepath.Join(dir, "bus")+`</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("dbus-daemon", "--config-file="+config, "--nofork", "--print-address")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	addr, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", strings.TrimSpace(addr))

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	p := &fakePortal{conn: conn, got: make(chan map[string]dbus.Variant, 1)}
	if err := conn.Export(p, portalPath, portalFileChooser); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.RequestName(portalDest, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPortalOpen(t *testing.T) {
	p := startFakePortal(t)
	p.set(0, "file:///home/me/My%20Notes/plan.md")
	got, err := PortalOpen("Open")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/home/me/My Notes/plan.md" {
		t.Errorf("got %q", got)
	}
	if _, ok := (<-p.got)["filters"]; !ok {
		t.Error("no file type filters sent")
	}

	p.set(1, "")
	got, err = PortalOpen("Open")
	if got != "" || err != nil {
		t.Errorf("cancelled dialog gave %q, %v", got, err)
	}
	<-p.got
}

func TestPortalSave(t *testing.T) {
	p := startFakePortal(t)
	p.set(0, "file:///tmp/out.md")
	got, err := PortalSave("Save", "/home/me/notes/plan.md")
	if err != nil || got != "/tmp/out.md" {
		t.Fatalf("got %q, %v", got, err)
	}
	opts := <-p.got
	if name, _ := opts["current_name"].Value().(string); name != "plan.md" {
		t.Errorf("current_name %q", name)
	}
	if dir, _ := opts["current_folder"].Value().([]byte); string(dir) != "/home/me/notes\x00" {
		t.Errorf("current_folder %q", dir)
	}

	if _, err := PortalSave("Save", ""); err != nil {
		t.Fatal(err)
	}
	if name, _ := (<-p.got)["current_name"].Value().(string); name != "Untitled.md" {
		t.Errorf("untitled current_name %q", name)
	}
}

// Without a portal on the bus the dialog fails with an error, not a hang.
func TestPortalMissing(t *testing.T) {
	startFakePortal(t).conn.ReleaseName(portalDest)
	if _, err := PortalOpen("Open"); err == nil {
		t.Error("no error without a portal")
	}
}

func TestURIPath(t *testing.T) {
	if _, err := uriPath("https://example.com/a.md"); err == nil {
		t.Error("a web URI was taken as a file")
	}
}
