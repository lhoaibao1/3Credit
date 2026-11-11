// Package tracking records outbound clicks and inbound conversion postbacks.
//
// Rules:
//   - The outbound URL carries only the click ID. Never PII or credit data.
//   - The same session clicking the same campaign within DedupeWindow reuses
//     the existing click ID.
//   - Postbacks must be signed (HMAC-SHA256 with the campaign's secret) and
//     are idempotent on (network, conversion_id).
package tracking

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/threecredit/platform/internal/model"
)

const DedupeWindow = 30 * time.Minute

type Tracker struct {
	DB   *sql.DB
	Now  func() time.Time
	Salt string // for IP hashing
}

func New(db *sql.DB, salt string) *Tracker { return &Tracker{DB: db, Now: time.Now, Salt: salt} }

type ClickMeta struct {
	Session, Source, Medium, Landing, Device, IP string
}

func newID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b)
}

// Click creates (or reuses) a click and returns the click ID and the partner URL.
func (t *Tracker) Click(o model.Offer, m ClickMeta) (string, string, error) {
	if !o.Campaign.Live(t.Now()) {
		return "", "", errors.New("campaign not live")
	}
	var id string
	since := t.Now().Add(-DedupeWindow).UTC()
	err := t.DB.QueryRow(`SELECT click_id FROM clicks WHERE session_id=? AND campaign_id=? AND created_at>=? ORDER BY created_at DESC LIMIT 1`,
		m.Session, o.Campaign.ID, since).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		id = newID("clk_")
		h := sha256.Sum256([]byte(t.Salt + m.IP))
		_, err = t.DB.Exec(`INSERT INTO clicks(click_id,campaign_id,partner_id,product_id,session_id,source,medium,landing,device,ip_hash,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			id, o.Campaign.ID, o.Partner.ID, o.Product.ID, m.Session, trunc(m.Source, 80), trunc(m.Medium, 80), trunc(m.Landing, 200), m.Device, hex.EncodeToString(h[:8]), t.Now().UTC())
	}
	if err != nil {
		return "", "", err
	}
	u, err := BuildURL(o.Campaign.TrackingURL, o.Campaign.SubIDParam, id)
	return id, u, err
}

// BuildURL inserts the click ID into a partner tracking URL. "{click_id}" is
// replaced if present; otherwise the SubID parameter is appended.
func BuildURL(base, param, clickID string) (string, error) {
	if strings.Contains(base, "{click_id}") {
		base = strings.ReplaceAll(base, "{click_id}", url.QueryEscape(clickID))
	} else {
		u, err := url.Parse(base)
		if err != nil {
			return "", err
		}
		if param == "" {
			param = "sub_id"
		}
		q := u.Query()
		q.Set(param, clickID)
		u.RawQuery = q.Encode()
		base = u.String()
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", errors.New("invalid tracking URL")
	}
	return base, nil
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ---------- postback ----------

// Sign returns the expected signature for a postback.
func Sign(secret, clickID, conversionID, status, payout string) string {
	m := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(m, "%s|%s|%s|%s", clickID, conversionID, status, payout)
	return hex.EncodeToString(m.Sum(nil))
}

// convStatus maps the status words affiliate networks send to our three states.
var convStatus = map[string]string{
	"pending": "pending", "hold": "pending", "new": "pending", "waiting": "pending",
	"approved": "approved", "approve": "approved", "success": "approved", "successful": "approved", "confirmed": "approved", "paid": "approved",
	"rejected": "rejected", "reject": "rejected", "cancelled": "rejected", "canceled": "rejected", "declined": "rejected", "failed": "rejected",
}

type PostbackResult struct {
	Result string // created | updated | duplicate
	Click  string
	Payout float64
}

// Postback verifies and records a conversion. q holds click_id, conversion_id,
// status, payout and sig.
func (t *Tracker) Postback(network string, q url.Values) (PostbackResult, error) {
	res, err := t.postback(network, q)
	ok := err == nil
	msg := res.Result
	if err != nil {
		msg = err.Error()
	}
	safe := url.Values{}
	for _, k := range []string{"click_id", "conversion_id", "status", "payout"} {
		safe.Set(k, trunc(q.Get(k), 100))
	}
	t.DB.Exec(`INSERT INTO postback_logs(network,query,signature_ok,result) VALUES(?,?,?,?)`, trunc(network, 40), safe.Encode(), ok, trunc(msg, 200))
	return res, err
}

func (t *Tracker) postback(network string, q url.Values) (PostbackResult, error) {
	clickID, convID, rawStatus, payoutS := q.Get("click_id"), q.Get("conversion_id"), q.Get("status"), q.Get("payout")
	sig, token := q.Get("sig"), q.Get("token")
	if network == "" || clickID == "" || convID == "" || (sig == "" && token == "") {
		return PostbackResult{}, errors.New("missing parameters")
	}
	status, ok := convStatus[strings.ToLower(strings.TrimSpace(rawStatus))]
	if !ok {
		return PostbackResult{}, errors.New("invalid status")
	}
	payout, err := strconv.ParseFloat(payoutS, 64)
	if payoutS == "" {
		payout, err = 0, nil
	}
	if err != nil || payout < 0 {
		return PostbackResult{}, errors.New("invalid payout")
	}
	var campaignID, secret string
	err = t.DB.QueryRow(`SELECT c.campaign_id, k.postback_secret FROM clicks c JOIN campaigns k ON k.id=c.campaign_id WHERE c.click_id=?`, clickID).Scan(&campaignID, &secret)
	if errors.Is(err, sql.ErrNoRows) {
		return PostbackResult{}, errors.New("unknown click_id")
	}
	if err != nil {
		return PostbackResult{}, err
	}
	if secret == "" {
		return PostbackResult{}, errors.New("campaign has no postback secret")
	}
	// Two ways to prove the call is genuine: the campaign's secret token in the URL (what affiliate
	// networks support), or an HMAC signature over the fields (for partners that can compute one).
	if token != "" {
		if !hmac.Equal([]byte(token), []byte(secret)) {
			return PostbackResult{}, errors.New("bad token")
		}
	} else if want := Sign(secret, clickID, convID, rawStatus, payoutS); !hmac.Equal([]byte(want), []byte(strings.ToLower(sig))) {
		return PostbackResult{}, errors.New("bad signature")
	}
	var oldStatus, oldClick string
	err = t.DB.QueryRow(`SELECT status, click_id FROM conversions WHERE network=? AND conversion_id=?`, network, convID).Scan(&oldStatus, &oldClick)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = t.DB.Exec(`INSERT INTO conversions(network,conversion_id,click_id,campaign_id,status,payout) VALUES(?,?,?,?,?,?)`,
			network, convID, clickID, campaignID, status, payout)
		return PostbackResult{Result: "created", Click: clickID, Payout: payout}, err
	case err != nil:
		return PostbackResult{}, err
	case oldClick != clickID:
		return PostbackResult{}, errors.New("conversion_id already bound to another click")
	case oldStatus == status:
		return PostbackResult{Result: "duplicate", Click: clickID, Payout: payout}, nil
	default:
		_, err = t.DB.Exec(`UPDATE conversions SET status=?, payout=?, updated_at=CURRENT_TIMESTAMP WHERE network=? AND conversion_id=?`, status, payout, network, convID)
		return PostbackResult{Result: "updated", Click: clickID, Payout: payout}, err
	}
}
