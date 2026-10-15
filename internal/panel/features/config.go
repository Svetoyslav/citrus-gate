// Package features contains the optional Citrus Gate features. All switches start off.
package features

import (
	"context"
	"errors"
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
	XrayAuto     bool    `json:"xray_auto"`
	AutoName     string  `json:"auto_name"`
	AdminIDs     []int64 `json:"admin_ids"`
}

func Load(ctx context.Context, set *settings.Settings) (Config, error) {
	c, _, err := settings.Get[Config](ctx, set, Key)
	if c.AutoName == "" {
		c.AutoName = "Авто"
	}
	return c, err
}
func (c Config) Validate() error {
	if c.ReferralDays < 0 || c.ReferralDays > 365 || c.DeviceStars < 0 || c.DeviceStars > 10000 || c.DeviceRub < 0 || c.DeviceRub > 100000000 || (c.DeviceRub > 0 && c.DeviceRub < 100) || c.GraceInbound < 0 || len(c.AdminIDs) > 20 {
		return errors.New("invalid_feature_settings")
	}
	for _, id := range c.AdminIDs {
		if id <= 0 {
			return errors.New("invalid_admin_id")
		}
	}
	return nil
}

// Grace never revives a disabled user and ends exactly 24 hours after expiry.
func (c Config) Grace(u db.User, now time.Time) bool {
	return c.GraceInbound > 0 && u.Status == "active" && u.ExpiresAt.Valid && now.Unix() >= u.ExpiresAt.Int64 && now.Unix() < u.ExpiresAt.Int64+86400
}
