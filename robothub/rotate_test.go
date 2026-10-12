package robothub

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/i41-eu/q41eu-raw/wire"
)

// The robot gets a fresh pairing code every PairTTL, and the code it shows works.
func TestPairCodeRotates(t *testing.T) {
	hub := New(Config{Token: "secret", PublicURL: "http://dev.test", PairTTL: 300 * time.Millisecond,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ts := httptest.NewServer(hub)
	defer ts.Close()
	robot, _, err := dial(t, ts.URL, "chan-1", "secret")
	if err != nil {
		t.Fatal(err)
	}
	robot.register()
	var first, next wire.PairCodeBody
	robot.next(wire.KindPairCode, &first)
	robot.next(wire.KindPairCode, &next)
	if next.Code == first.Code {
		t.Fatal("no fresh code")
	}
	time.Sleep(100 * time.Millisecond)
	if _, ok := hub.Redeem(next.Code); !ok {
		t.Fatal("the code the robot got last does not work")
	}
}
