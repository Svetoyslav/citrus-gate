// Package citrusbot adds the Citrus Gate commands to the existing subscriber bot.
package citrusbot

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"mikan/internal/panel/audit"
	"mikan/internal/panel/billing"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/events"
	"mikan/internal/panel/features"
	"mikan/internal/panel/secure"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store"
	"mikan/internal/panel/tgate"
	"mikan/internal/panel/tgbot"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type confirmation struct {
	chat   int64
	action string
	id     int64
	until  time.Time
}
type Handler struct {
	st           *store.Store
	set          *settings.Settings
	g            *tgate.Gateway
	bill         *billing.Service
	users        *domain.Users
	devices      *domain.Devices
	now          func() time.Time
	log          *slog.Logger
	backend      tgbot.Backend
	nodesChanged func()
	mu           sync.Mutex
	pending      map[string]confirmation
	bot          *tgbot.Bot
	client       *tgate.Client
}

func New(st *store.Store, set *settings.Settings, g *tgate.Gateway, bill *billing.Service, users *domain.Users, devices *domain.Devices, backend tgbot.Backend, now func() time.Time, log *slog.Logger) *Handler {
	return &Handler{st: st, set: set, g: g, bill: bill, users: users, devices: devices, backend: backend, now: now, log: log, pending: map[string]confirmation{}}
}
func (h *Handler) SetNodesChanged(f func()) { h.nodesChanged = f }
func (h *Handler) screens() *tgbot.Bot {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.g.Client()
	if c == nil || h.g.Outbox() == nil {
		return nil
	}
	if h.bot == nil || h.client != c {
		h.client = c
		h.bot = tgbot.New(tgbot.Deps{Backend: h.backend, Client: c, Outbox: h.g.Outbox(), Go: h.g.Go, Log: h.log, Now: h.now})
	}
	return h.bot
}
func (h *Handler) send(chat int64, text string, rows ...[]tgate.Button) {
	if out := h.g.Outbox(); out != nil {
		out.Reply(chat, "cg-reply", 1, func(ctx context.Context, c *tgate.Client) error {
			_, e := c.Send(ctx, chat, text, &tgate.Keyboard{InlineKeyboard: rows}, false)
			return e
		})
	}
}
func btn(text, data string) tgate.Button { return tgate.Button{Text: text, CallbackData: "cg:" + data} }
func (h *Handler) Update(ctx context.Context, up tgate.Update) error {
	var chat int64
	data := ""
	if q := up.CallbackQuery; q != nil && q.Message != nil && strings.HasPrefix(q.Data, "cg:") {
		if q.From.ID != q.Message.Chat.ID {
			return nil
		}
		chat = q.From.ID
		data = strings.TrimPrefix(q.Data, "cg:")
		if c := h.g.Client(); c != nil {
			h.g.Go(func() { _ = c.Answer(ctx, q.ID, "") })
		}
	} else if m := up.Message; m != nil && m.From != nil {
		chat = m.Chat.ID
		words := strings.Fields(m.Text)
		if len(words) > 0 {
			cmd := strings.Split(words[0], "@")[0]
			switch cmd {
			case "/start":
				if len(words) == 2 && strings.HasPrefix(words[1], "ref_") {
					if e := h.bill.RegisterReferral(ctx, chat, strings.TrimPrefix(words[1], "ref_")); e != nil {
						h.log.Debug("referral rejected", "account", chat)
					}
					copy := *m
					copy.Text = "/start"
					up.Message = &copy
				}
			case "/admin":
				data = "admin"
			case "/ref":
				data = "ref"
			case "/devicesplus":
				data = "devices"
			case "/user":
				if len(words) == 2 {
					data = "user:" + words[1]
				}
			}
		}
	}
	if data == "" {
		if b := h.screens(); b != nil {
			b.Update(ctx, up)
		}
		return nil
	}
	cfg, e := features.Load(ctx, h.set)
	if e != nil {
		return e
	}
	parts := strings.Split(data, ":")
	action := parts[0]
	id := int64(0)
	if len(parts) > 1 {
		id, _ = strconv.ParseInt(parts[1], 10, 64)
	}
	switch action {
	case "ref", "devices":
		links, e := h.st.Q.ListTgLinksOf(ctx, chat)
		if e != nil {
			return e
		}
		rows := [][]tgate.Button{}
		for _, u := range links {
			next := "buy:"
			if action == "ref" {
				next = "refuser:"
			}
			rows = append(rows, []tgate.Button{btn(u.Name, next+strconv.FormatInt(u.ID, 10))})
		}
		h.send(chat, "Выберите подписку. Реферальная программа: /ref · Дополнительное устройство: /devicesplus", rows...)
		return nil
	case "refuser", "buy", "pay":
		owner, e := h.st.Q.GetTgLink(ctx, id)
		if e != nil || owner.TgID != chat {
			h.send(chat, "Подписка недоступна.")
			return nil
		}
		if action == "refuser" {
			if cfg.ReferralDays == 0 {
				h.send(chat, "Реферальная программа выключена.")
				return nil
			}
			code, e := h.bill.ReferralCode(ctx, id)
			if e != nil {
				return e
			}
			link := h.g.BotURL(ctx) + "?start=ref_" + code
			h.send(chat, fmt.Sprintf("За первую покупку приглашённого друга: +%d дней.\n%s", cfg.ReferralDays, html.EscapeString(link)))
			return nil
		}
		if action == "pay" {
			if len(parts) != 3 {
				return nil
			}
			p, e := h.bill.DeviceInvoice(ctx, chat, id, billing.AdapterOf(parts[2]))
			if e != nil {
				h.send(chat, "Счёт не создан. Проверьте срок подписки и доступность оплаты.")
				return nil
			}
			h.send(chat, "+1 устройство без изменения срока подписки.", []tgate.Button{{Text: "Оплатить", URL: p.PayUrl}})
			return nil
		}
		o, e := h.bill.DeviceOffer(ctx, id)
		if e != nil {
			return e
		}
		rows := [][]tgate.Button{}
		if o.Stars > 0 {
			rows = append(rows, []tgate.Button{btn(fmt.Sprintf("%d Stars", o.Stars), fmt.Sprintf("pay:%d:stars", id))})
		}
		if o.Rub > 0 {
			for _, a := range o.Available.Addons {
				rows = append(rows, []tgate.Button{btn(fmt.Sprintf("%.2f ₽ · %s", float64(o.Rub)/100, h.bill.AddonName(ctx, a, "ru")), fmt.Sprintf("pay:%d:%s", id, a))})
			}
		}
		text := "Купить +1 устройство. Срок подписки не меняется."
		if len(rows) == 0 {
			text = "Покупка устройств недоступна: подписка закончилась, лимит не установлен или продажа выключена."
		}
		h.send(chat, text, rows...)
		return nil
	}
	if !slices.Contains(cfg.AdminIDs, chat) {
		h.send(chat, "Нет доступа к административным командам.")
		return nil
	}
	switch action {
	case "admin":
		h.send(chat, "Citrus Gate · Администрирование\n/user ID — открыть клиента.", []tgate.Button{btn("Клиенты", "users:0"), btn("Ноды", "nodes")}, []tgate.Button{btn("Тарифы", "tariffs")})
	case "users":
		rows, e := h.st.DB.QueryContext(ctx, "SELECT id,name,status FROM users WHERE id>$1 ORDER BY id LIMIT 21", id)
		if e != nil {
			return e
		}
		defer rows.Close()
		var buttons [][]tgate.Button
		last := id
		n := 0
		for rows.Next() {
			var uid int64
			var name, status string
			if e := rows.Scan(&uid, &name, &status); e != nil {
				return e
			}
			n++
			if n > 20 {
				break
			}
			last = uid
			buttons = append(buttons, []tgate.Button{btn(fmt.Sprintf("%d · %s · %s", uid, name, status), fmt.Sprintf("user:%d", uid))})
		}
		if n > 20 {
			buttons = append(buttons, []tgate.Button{btn("Далее", fmt.Sprintf("users:%d", last))})
		}
		h.send(chat, "Клиенты", buttons...)
	case "user":
		u, e := h.st.Q.GetUser(ctx, id)
		if e != nil {
			h.send(chat, "Клиент не найден.")
			return nil
		}
		expiry := "без срока"
		if u.ExpiresAt.Valid {
			expiry = time.Unix(u.ExpiresAt.Int64, 0).UTC().Format("02.01.2006 15:04 UTC")
		}
		h.send(chat, fmt.Sprintf("%d · %s\nСтатус: %s\nСрок: %s", id, html.EscapeString(u.Name), html.EscapeString(u.Status), expiry), []tgate.Button{btn("+7 дней", fmt.Sprintf("askextend:%d", id)), btn("Вкл./выкл.", fmt.Sprintf("asktoggle:%d", id))}, []tgate.Button{btn("Устройства", fmt.Sprintf("bound:%d", id)), btn("Тариф", fmt.Sprintf("picktariff:%d", id))})
	case "bound":
		ds, e := h.st.Q.ListBoundDevices(ctx, id)
		if e != nil {
			return e
		}
		rows := [][]tgate.Button{}
		for _, d := range ds {
			rows = append(rows, []tgate.Button{btn(fmt.Sprintf("Отвязать #%d", d.ID), fmt.Sprintf("askunbind:%d:%d", id, d.ID))})
		}
		h.send(chat, fmt.Sprintf("Устройства клиента %d", id), rows...)
	case "picktariff":
		ts, e := h.st.Q.ListTariffs(ctx)
		if e != nil {
			return e
		}
		rows := [][]tgate.Button{}
		for _, t := range ts {
			if t.Archived == 0 {
				rows = append(rows, []tgate.Button{btn(t.Name, fmt.Sprintf("asktariff:%d:%d", id, t.ID))})
			}
		}
		h.send(chat, "Выберите тариф. Срок подписки сохранится.", rows...)
	case "nodes":
		ns, e := h.st.Q.ListNodes(ctx)
		if e != nil {
			return e
		}
		text := "Ноды\n"
		rows := [][]tgate.Button{}
		for _, n := range ns {
			text += fmt.Sprintf("%d · %s · %s · enabled=%d\n", n.ID, html.EscapeString(n.Name), html.EscapeString(n.PublicHost), n.Enabled)
			if n.ID > 1 {
				rows = append(rows, []tgate.Button{btn(fmt.Sprintf("Вкл./выкл. %s", n.Name), fmt.Sprintf("asknodetoggle:%d", n.ID))})
			}
		}
		h.send(chat, text, rows...)
	case "tariffs":
		ts, e := h.st.Q.ListTariffs(ctx)
		if e != nil {
			return e
		}
		text := "Тарифы\n"
		for _, t := range ts {
			text += fmt.Sprintf("%d · %s · %d дней\n", t.ID, html.EscapeString(t.Name), t.DurationDays)
		}
		h.send(chat, text)
	case "askextend", "asktoggle", "askunbind", "asktariff", "asknodetoggle":
		key := secure.Token(12)
		act := action
		if action == "askunbind" || action == "asktariff" {
			if len(parts) != 3 {
				return nil
			}
			act += "_" + parts[2]
		}
		h.mu.Lock()
		for k, v := range h.pending {
			if h.now().After(v.until) {
				delete(h.pending, k)
			}
		}
		h.pending[key] = confirmation{chat, act, id, h.now().Add(5 * time.Minute)}
		h.mu.Unlock()
		label := map[string]string{"askextend": "добавить 7 дней", "asktoggle": "изменить доступ клиента", "askunbind": "отвязать устройство", "asktariff": "сменить тариф, сохранив срок", "asknodetoggle": "включить или отключить удалённую ноду"}[action]
		h.send(chat, fmt.Sprintf("Подтвердите: %s. ID: %d.", label, id), []tgate.Button{btn("Подтвердить", "confirm:"+key), btn("Отмена", "admin")})
	case "confirm":
		if len(parts) != 2 {
			return nil
		}
		h.mu.Lock()
		p, ok := h.pending[parts[1]]
		if ok && p.chat == chat {
			delete(h.pending, parts[1])
		}
		h.mu.Unlock()
		if !ok || p.chat != chat || !h.now().Before(p.until) {
			h.send(chat, "Подтверждение истекло.")
			return nil
		}
		var e error
		switch {
		case p.action == "asknodetoggle":
			if p.id <= 1 {
				return nil
			}
			_, e = h.st.DB.ExecContext(ctx, "UPDATE nodes SET enabled=CASE WHEN enabled=1 THEN 0 ELSE 1 END,updated_at=$2 WHERE id=$1", p.id, h.now().Unix())
			if e == nil && h.nodesChanged != nil {
				h.nodesChanged()
			}
		case strings.HasPrefix(p.action, "asktariff_"):
			tid, _ := strconv.ParseInt(strings.TrimPrefix(p.action, "asktariff_"), 10, 64)
			u, err := h.st.Q.GetUser(ctx, p.id)
			e = err
			if e == nil {
				patch := domain.Patch{TariffID: &tid, ClearExpiry: !u.ExpiresAt.Valid}
				if u.ExpiresAt.Valid {
					expiry := time.Unix(u.ExpiresAt.Int64, 0)
					patch.ExpiresAt = &expiry
				}
				_, e = h.users.Update(ctx, p.id, patch)
			}
		case p.action == "askextend":
			_, e = h.users.Extend(ctx, p.id, 7)
		case p.action == "asktoggle":
			u, err := h.st.Q.GetUser(ctx, p.id)
			e = err
			if e == nil {
				disabled := u.Status != "disabled"
				_, e = h.users.Update(ctx, p.id, domain.Patch{Disabled: &disabled})
			}
		case strings.HasPrefix(p.action, "askunbind_"):
			dev, _ := strconv.ParseInt(strings.TrimPrefix(p.action, "askunbind_"), 10, 64)
			e = h.devices.Unbind(ctx, p.id, dev, false)
		}
		if e != nil {
			h.send(chat, "Действие не выполнено. Обновите данные и повторите.")
			return nil
		}
		targetType := "user"
		if p.action == "asknodetoggle" {
			targetType = "node"
		}
		_ = audit.Write(ctx, h.st.Q, h.now(), audit.Entry{Action: "citrus.telegram." + p.action, TargetType: targetType, TargetID: strconv.FormatInt(p.id, 10), Details: map[string]any{"telegram_admin": chat}})
		if p.action == "asknodetoggle" {
			h.send(chat, "Готово.", []tgate.Button{btn("К нодам", "nodes")})
		} else {
			h.send(chat, "Готово.", []tgate.Button{btn("К клиенту", fmt.Sprintf("user:%d", p.id))})
		}
	}
	return nil
}

// Run forwards committed payment/notification events to the built-in screens, keeping
// a durable cursor. Telegram delivery is at least once across a process crash.
func (h *Handler) Run(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		b := h.screens()
		if b == nil {
			continue
		}
		cursor, _, e := settings.Get[int64](ctx, h.set, "citrus_bot_cursor")
		if e != nil {
			continue
		}
		rows, e := h.st.DB.QueryContext(ctx, "SELECT id,type,user_id,tg_id,data,created_at FROM addon_events WHERE id>$1 ORDER BY id LIMIT 100", cursor)
		if e != nil {
			continue
		}
		batch := []events.Event{}
		for rows.Next() {
			var ev events.Event
			var uid, tid *int64
			var raw string
			var at int64
			if rows.Scan(&ev.ID, &ev.Type, &uid, &tid, &raw, &at) != nil {
				break
			}
			if uid != nil {
				ev.UserID = *uid
			}
			if tid != nil {
				ev.TgID = *tid
			}
			ev.Data = json.RawMessage(raw)
			ev.At = time.Unix(at, 0)
			batch = append(batch, ev)
		}
		rows.Close()
		for _, ev := range batch {
			if slices.Contains(tgbot.EventTypes, ev.Type) {
				if e := b.HandleEvent(ctx, ev); e != nil {
					break
				}
			}
			if settings.Set(ctx, h.set, "citrus_bot_cursor", ev.ID) != nil {
				break
			}
		}
	}
}
