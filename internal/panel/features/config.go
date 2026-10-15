// Package features contains the optional Citrus Gate features. All switches start off.
package features

import (
	"context"
	"errors"
	"math"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"time"
)

const Key = "citrus_features"

type Config struct {
	ReferralDays int64   `json:"referral_days"`
	DeviceStars  int64   `json:"device_stars"`
	DeviceRub    int64   `json:"device_rub"`
	GraceInbound int64   `json:"grace_inbound"`
	GraceDays    int64   `json:"grace_days"`
	XrayAuto     bool    `json:"xray_auto"`
	AutoName     string  `json:"auto_name"`
	AdminIDs     []int64 `json:"admin_ids"`
}

func Load(ctx context.Context, set *settings.Settings) (Config, error) {
	// Preserve the previous one-day window for saved configurations without this field.
	c, _, err := settings.GetOver(ctx, set, Key, Config{GraceDays: 1})
	if c.AutoName == "" {
		c.AutoName = "Авто"
	}
	return c, err
}
func (c Config) Validate() error {
	if c.ReferralDays < 0 || c.ReferralDays > 365 || c.DeviceStars < 0 || c.DeviceStars > 10000 || c.DeviceRub < 0 || c.DeviceRub > 100000000 || (c.DeviceRub > 0 && c.DeviceRub < 100) || c.GraceInbound < 0 || c.GraceDays < 0 || c.GraceDays > math.MaxInt64/86400 || len(c.AdminIDs) > 20 {
		return errors.New("invalid_feature_settings")
	}
	for _, id := range c.AdminIDs {
		if id <= 0 {
			return errors.New("invalid_admin_id")
		}
	}
	return nil
}

// GraceUntil is shared by the subscription response and the node policy. Zero disables
// renewal access. A day is 86400 seconds after subscription expiry, independent of DST.
func (c Config) GraceUntil(u db.User) int64 {
	if c.GraceInbound <= 0 || c.GraceDays <= 0 || c.GraceDays > math.MaxInt64/86400 || u.Status != "active" || !u.ExpiresAt.Valid {
		return 0
	}
	seconds := c.GraceDays * 86400
	if u.ExpiresAt.Int64 > math.MaxInt64-seconds {
		return 0
	}
	return u.ExpiresAt.Int64 + seconds
}

// Grace never revives a disabled user and respects the administrator's window.
func (c Config) Grace(u db.User, now time.Time) bool {
	until := c.GraceUntil(u)
	return until > 0 && now.Unix() >= u.ExpiresAt.Int64 && now.Unix() < until
}
