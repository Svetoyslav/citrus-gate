package billing

import (
	"context"
	"database/sql"
	"errors"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/features"
	"mikan/internal/panel/secure"
	"mikan/internal/panel/store/db"
)

const KindDevice = "device"

type DeviceOffer struct {
	Stars     int64     `json:"stars"`
	Rub       int64     `json:"rub"`
	Available Available `json:"available"`
}

func (s *Service) DeviceOffer(ctx context.Context, id int64) (DeviceOffer, error) {
	c, err := features.Load(ctx, s.d.Settings)
	if err != nil {
		return DeviceOffer{}, err
	}
	u, err := s.d.Store.Q.GetUser(ctx, id)
	if err != nil {
		return DeviceOffer{}, err
	}
	if !u.DeviceLimit.Valid || u.DeviceLimit.Int64 >= domain.MaxDevices || u.Status != "active" || u.ExpiresAt.Valid && u.ExpiresAt.Int64 <= s.d.Now().Unix() {
		return DeviceOffer{}, nil
	}
	av := s.Available(ctx)
	stars, rub := av.prices(devicePrice(c.DeviceStars), devicePrice(c.DeviceRub))
	return DeviceOffer{stars, rub, av}, nil
}
func (s *Service) DeviceInvoice(ctx context.Context, tgID, userID int64, provider string) (db.Payment, error) {
	link, err := s.d.Store.Q.GetTgLink(ctx, userID)
	if err != nil || link.TgID != tgID {
		return db.Payment{}, ErrNotYours
	}
	offer, err := s.DeviceOffer(ctx, userID)
	if err != nil {
		return db.Payment{}, err
	}
	amount, currency, ok := offer.Available.price(provider, devicePrice(offer.Stars), devicePrice(offer.Rub))
	if !ok {
		return db.Payment{}, ErrNotForSale
	}
	unlock, err := s.lockBuyer(ctx, tgID)
	if err != nil {
		return db.Payment{}, err
	}
	defer unlock()
	now := s.d.Now()
	pay, open, err := s.newPayment(ctx, tgID, now, func(q *db.Queries) ([]db.Payment, error) {
		// CreatePayment and the standard invoice limiter keep this on the existing billing path.
		return q.CitrusOpenDevices(ctx, tgID, userID, provider, now.Add(-invoiceReuse).Unix())
	}, func(q *db.Queries, p db.Payment) (bool, error) {
		return p.Amount == amount && p.Currency == currency, nil
	}, func(q *db.Queries) (db.Payment, error) {
		return q.CreatePayment(ctx, db.CreatePaymentParams{Provider: provider, Payload: secure.Token(32), TgID: tgID, Kind: KindDevice, UserID: sql.NullInt64{Int64: userID, Valid: true}, TariffName: "+1 устройство", Amount: amount, Currency: currency, CreatedAt: now.Unix()})
	})
	if err != nil || open {
		return pay, err
	}
	return s.openPayment(ctx, pay, "+1 устройство", "Одно дополнительное устройство без продления подписки")
}
func (s *Service) applyDevice(ctx context.Context, q *db.Queries, p db.Payment) (db.User, error) {
	if !p.UserID.Valid {
		return db.User{}, errUserGone
	}
	u, err := q.GetUser(ctx, p.UserID.Int64)
	if err != nil {
		return u, err
	}
	// A payment may arrive after expiration or after an admin changed the tariff. Keep
	// the entitlement even on an unlimited tariff; a later finite tariff includes it.
	res, err := q.CitrusSQL().ExecContext(ctx, "INSERT INTO cg_device_grants(payment_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING", p.ID, u.ID)
	if err != nil {
		return u, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		_, err = q.CitrusSQL().ExecContext(ctx, "UPDATE users SET device_limit=device_limit+1,updated_at=$2 WHERE id=$1", u.ID, s.d.Now().Unix())
	}
	if err != nil {
		return u, err
	}
	return q.GetUser(ctx, u.ID)
}

// ReferralCode is a stable, unguessable bot deep-link code for one subscription.
func (s *Service) ReferralCode(ctx context.Context, userID int64) (string, error) {
	code := secure.Token(18)
	var out string
	err := s.d.Store.Q.CitrusSQL().QueryRowContext(ctx, "INSERT INTO cg_referral_codes(user_id,code) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET user_id=EXCLUDED.user_id RETURNING code", userID, code).Scan(&out)
	return out, err
}
func (s *Service) RegisterReferral(ctx context.Context, tgID int64, code string) error {
	c, e := features.Load(ctx, s.d.Settings)
	if e != nil {
		return e
	}
	if c.ReferralDays <= 0 {
		return ErrNotForSale
	}
	if tgID <= 0 || len(code) > 64 || code == "" {
		return ErrBadPayment
	}
	return s.d.Store.Tx(ctx, func(q *db.Queries) error {
		var owner int64
		err := q.CitrusSQL().QueryRowContext(ctx, "SELECT user_id FROM cg_referral_codes WHERE code=$1", code).Scan(&owner)
		if err != nil {
			return err
		}
		// Neither existing subscribers nor previous buyers count as newly invited people.
		var known bool
		err = q.CitrusSQL().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM tg_links WHERE tg_id=$1) OR EXISTS(SELECT 1 FROM payments WHERE tg_id=$1 AND status IN ('paid','applied','refunded')) OR EXISTS(SELECT 1 FROM tg_links WHERE user_id=$2 AND tg_id=$1)", tgID, owner).Scan(&known)
		if err != nil || known {
			return err
		}
		_, err = q.CitrusSQL().ExecContext(ctx, "INSERT INTO cg_referrals(invitee,referrer,created_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", tgID, owner, s.d.Now().Unix())
		return err
	})
}
func (s *Service) rewardReferral(ctx context.Context, q *db.Queries, p db.Payment) error {
	if p.Kind != "new" && p.Kind != "renew" || p.Amount <= 0 {
		return nil
	}
	c, err := features.Load(ctx, s.d.Settings)
	if err != nil {
		return err
	}
	if c.ReferralDays <= 0 {
		return nil
	}
	var owner int64
	err = q.CitrusSQL().QueryRowContext(ctx, "SELECT referrer FROM cg_referrals WHERE invitee=$1 AND referrer IS NOT NULL AND payment_id IS NULL FOR UPDATE", p.TgID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// A subscription later transferred to the invitee cannot reward itself.
	link, e := q.GetTgLink(ctx, owner)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if e == nil && link.TgID == p.TgID {
		return nil
	}
	var seconds int64
	err = q.CitrusSQL().QueryRowContext(ctx, "UPDATE users SET expires_at=GREATEST(expires_at,$2)+$3,updated_at=$2 WHERE id=$1 AND expires_at IS NOT NULL RETURNING $3::bigint", owner, s.d.Now().Unix(), c.ReferralDays*86400).Scan(&seconds)
	if errors.Is(err, sql.ErrNoRows) {
		seconds = 0
	} else if err != nil {
		return err
	}
	_, err = q.CitrusSQL().ExecContext(ctx, "UPDATE cg_referrals SET payment_id=$2,seconds=$3 WHERE invitee=$1", p.TgID, p.ID, seconds)
	return err
}
func (s *Service) reverseReferral(ctx context.Context, q *db.Queries, p db.Payment) error {
	var owner sql.NullInt64
	var seconds int64
	err := q.CitrusSQL().QueryRowContext(ctx, "UPDATE cg_referrals SET reversed=TRUE WHERE payment_id=$1 AND NOT reversed RETURNING referrer,seconds", p.ID).Scan(&owner, &seconds)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if owner.Valid && seconds > 0 {
		_, err = q.CitrusSQL().ExecContext(ctx, "UPDATE users SET expires_at=GREATEST($2,expires_at-$3),updated_at=$2 WHERE id=$1 AND expires_at IS NOT NULL", owner.Int64, s.d.Now().Unix(), seconds)
	}
	return err
}

func devicePrice(n int64) sql.NullInt64 { return sql.NullInt64{Int64: n, Valid: n > 0} }
