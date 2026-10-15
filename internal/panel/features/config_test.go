package features

import (
	"database/sql"
	"mikan/internal/panel/store/db"
	"testing"
	"time"
)

func TestCitrusGraceBoundaries(t *testing.T) {
	c := Config{GraceInbound: 7}
	u := db.User{Status: "active", ExpiresAt: sql.NullInt64{Int64: 100000, Valid: true}}
	for _, v := range []struct {
		at   int64
		want bool
	}{{99999, false}, {100000, true}, {186399, true}, {186400, false}} {
		if got := c.Grace(u, time.Unix(v.at, 0)); got != v.want {
			t.Fatalf("at %d got %v", v.at, got)
		}
	}
	u.Status = "disabled"
	if c.Grace(u, time.Unix(100001, 0)) {
		t.Fatal("disabled user revived")
	}
}
