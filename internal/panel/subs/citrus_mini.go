package subs

import (
	"encoding/json"
	"mikan/internal/panel/billing"
	"net/http"
)

type citrusRequest struct {
	miniAuth
	Token    string `json:"token"`
	Provider string `json:"provider"`
}

func (h *Handler) miniCitrus(w http.ResponseWriter, r *http.Request, rest string) {
	var in citrusRequest
	tgID, users, fail, ok := h.mini(w, r, &in)
	if !ok {
		return
	}
	id, ok := ownerOf(users, in.Token)
	if !ok || id == 0 {
		fail(403, "not_yours")
		return
	}
	if rest == "device-pay" {
		p, err := h.shop.DeviceInvoice(r.Context(), tgID, id, billing.AdapterOf(in.Provider))
		if err != nil {
			fail(409, "not_for_sale")
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"url": p.PayUrl})
		return
	}
	offer, err := h.shop.DeviceOffer(r.Context(), id)
	if err != nil {
		fail(500, "internal")
		return
	}
	cfg, err := h.cfg(r.Context())
	if err != nil {
		fail(500, "internal")
		return
	}
	ref := ""
	if cfg.Features.ReferralDays > 0 {
		code, err := h.shop.ReferralCode(r.Context(), id)
		if err != nil {
			fail(500, "internal")
			return
		}
		ref = BotURL(h.tg.LinkURL(r.Context(), id))
		if ref != "" {
			ref += "?start=ref_" + code
		}
	}
	var invited, rewarded int64
	if err := h.st.DB.QueryRowContext(r.Context(), "SELECT count(*),count(payment_id) FILTER (WHERE NOT reversed) FROM cg_referrals WHERE referrer=$1", id).Scan(&invited, &rewarded); err != nil {
		fail(500, "internal")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"device": offer, "referral_url": ref, "referral_days": cfg.Features.ReferralDays, "invited": invited, "rewarded": rewarded})
}
