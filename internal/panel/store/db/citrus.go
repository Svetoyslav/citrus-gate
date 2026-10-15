// Handwritten extension queries; sqlc generated files are left unchanged.
package db

import (
	"context"
	"database/sql"
)

func (q *Queries) CitrusSQL() DBTX { return q.db }

func (q *Queries) CitrusOpenDevices(ctx context.Context, tgID, userID int64, provider string, since int64) ([]Payment, error) {
	rows, err := q.db.QueryContext(ctx, "SELECT id FROM payments WHERE tg_id=$1 AND user_id=$2 AND provider=$3 AND kind='device' AND status='pending' AND pay_url<>'' AND created_at>$4 ORDER BY id DESC", tgID, userID, provider, since)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []Payment{}
	for _, id := range ids {
		p, e := q.GetPayment(ctx, id)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, nil
}
func (q *Queries) CitrusDeviceBonus(ctx context.Context, id int64, limit sql.NullInt64) (sql.NullInt64, error) {
	if !limit.Valid {
		return limit, nil
	}
	var n int64
	err := q.db.QueryRowContext(ctx, "SELECT count(*) FROM cg_device_grants WHERE user_id=$1", id).Scan(&n)
	limit.Int64 += n
	return limit, err
}

type CitrusSource struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url,omitempty"`
	Enabled     bool   `json:"enabled"`
	TariffIDs   string `json:"tariff_ids"`
	Interval    int64  `json:"interval_minutes"`
	Body        string `json:"-"`
	FetchedAt   int64  `json:"fetched_at"`
	AttemptedAt int64  `json:"attempted_at"`
	Error       string `json:"error"`
}

func (q *Queries) CitrusSources(ctx context.Context) ([]CitrusSource, error) {
	rows, err := q.db.QueryContext(ctx, "SELECT id,name,url,enabled,tariff_ids,interval_minutes,body,fetched_at,attempted_at,error FROM cg_external_sources ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CitrusSource{}
	for rows.Next() {
		var s CitrusSource
		if err := rows.Scan(&s.ID, &s.Name, &s.URL, &s.Enabled, &s.TariffIDs, &s.Interval, &s.Body, &s.FetchedAt, &s.AttemptedAt, &s.Error); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
