package internal_test

import (
	"context"
	"github.com/galaxytty/galaxytty/internal/app"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/mock"
	"github.com/galaxytty/galaxytty/internal/provider"
	"github.com/galaxytty/galaxytty/internal/scrcpy"
	"github.com/galaxytty/galaxytty/internal/tui"
	"testing"
)

func TestParserEdgeCases(t *testing.T) {
	in := "Row: 0 _id=12, body=한글, a=b 😀\n둘째 줄, type=2\n"
	r, e := provider.ParseContentRows(in, []string{"_id", "body", "type"})
	if e != nil || r[0]["body"] != "한글, a=b 😀\n둘째 줄" {
		t.Fatalf("%v %#v", e, r)
	}
}
func TestPhoneAndDisplay(t *testing.T) {
	if v := domain.NormalizePhone("+82 10-1234-5678"); v != "01012345678" {
		t.Fatal(v)
	}
	id, e := scrcpy.ParseDisplayID("[server] INFO: New display: 1080x1920/344 (id=18)")
	if e != nil || id != 18 {
		t.Fatal(id, e)
	}
}
func TestPoller(t *testing.T) {
	b := mock.New()
	p := app.Poller{Store: b}
	m, e := p.Poll(context.Background())
	if e != nil || len(m) != 0 || p.LastSeenID != 4 {
		t.Fatal(len(m), p.LastSeenID, e)
	}
	b.SimulateIncoming(1, "새 메시지")
	m, _ = p.Poll(context.Background())
	if len(m) != 1 || m[0].ID != 5 {
		t.Fatalf("%+v", m)
	}
	m, _ = p.Poll(context.Background())
	if len(m) != 0 {
		t.Fatal(len(m))
	}
}
func TestLifecycleShutdownIdempotent(t *testing.T) {
	d := &mock.Display{}
	n := &mock.Notifier{}
	l := app.NewLifecycle(d, n)
	if e := l.Transition(app.Connecting); e != nil {
		t.Fatal(e)
	}
	if e := l.Transition(app.Ready); e != nil {
		t.Fatal(e)
	}
	_ = l.Shutdown(context.Background())
	_ = l.Shutdown(context.Background())
	if d.Stops != 1 || n.Closed != 1 || l.State() != app.Disconnected {
		t.Fatal(d.Stops, n.Closed, l.State())
	}
}
func TestCommandsDefaultsAndSender(t *testing.T) {
	c := config.Default()
	if c.Polling.Interval.Duration.String() != "1s" || !c.Connection.PreferUSB {
		t.Fatal(c)
	}
	x, e := tui.ParseCommand("/search 안녕 세상")
	if e != nil || len(x.Args) != 2 {
		t.Fatal(x, e)
	}
	b := mock.New()
	if _, e = b.Send(context.Background(), "010-1234-5678", "안녕 😀"); e != nil || len(b.Sent) != 1 {
		t.Fatal(e)
	}
}
func TestDirectionMapping(t *testing.T) {
	v, e := provider.DirectionFromAndroid(2)
	if e != nil || domain.MessageDirection(v) != domain.DirectionOutgoing {
		t.Fatal(v, e)
	}
}
