package features

import (
	"database/sql"
	"math"
	"mikan/internal/panel/store/db"
	"testing"
	"time"
)

func TestCitrusGraceBoundaries(t *testing.T) {
	c := Config{GraceInbound: 7, GraceDays: 5}
	u := db.User{Status: "active", ExpiresAt: sql.NullInt64{Int64: 100000, Valid: true}}
	for _, v := range []struct {
		at   int64
		want bool
	}{{99999, false}, {100000, true}, {186400, true}, {531999, true}, {532000, false}} {
		if got := c.Grace(u, time.Unix(v.at, 0)); got != v.want {
			t.Fatalf("at %d got %v", v.at, got)
		}
	}
	u.Status = "disabled"
	if c.Grace(u, time.Unix(100001, 0)) {
		t.Fatal("disabled user revived")
	}
}

func TestCitrusGraceDisabledAndOverflow(t *testing.T) {
	u := db.User{Status: "active", ExpiresAt: sql.NullInt64{Int64: 100000, Valid: true}}
	for _, c := range []Config{{GraceInbound: 7}, {GraceDays: 5}, {GraceInbound: 7, GraceDays: -1}, {GraceInbound: 7, GraceDays: math.MaxInt64}} {
		if c.GraceUntil(u) != 0 || c.Grace(u, time.Unix(100001, 0)) {
			t.Fatalf("unexpected access: %+v", c)
		}
	}
	u.ExpiresAt.Int64 = math.MaxInt64 - 1
	if (Config{GraceInbound: 7, GraceDays: 5}).GraceUntil(u) != 0 {
		t.Fatal("timestamp overflow admitted")
	}
}
