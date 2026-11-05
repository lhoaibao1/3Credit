// Package leads stores consented leads (encrypted) and routes them to the
// endpoint configured on the campaign, with retries and idempotency keys.
//
// A lead is only created with an explicit consent naming the single
// recipient partner. 3Credit never fans a lead out to several partners.
package leads

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/threecredit/platform/internal/model"
)

const ConsentTextVersion = "consent-v1-2026-11"

type Router struct {
	DB     *sql.DB
	key    []byte
	Client *http.Client
	Now    func() time.Time
	MaxTry int
}

// New derives a 32-byte AES key from the configured secret.
func New(db *sql.DB, secret string) *Router {
	k := sha256.Sum256([]byte(secret))
	return &Router{DB: db, key: k[:], Client: &http.Client{Timeout: 10 * time.Second}, Now: time.Now, MaxTry: 5}
}

type Payload struct {
	Name   string        `json:"name"`
	Phone  string        `json:"phone"`
	Amount float64       `json:"amount"`
	Term   int           `json:"term"`
	Profile model.Profile `json:"profile"`
}

var phoneRe = regexp.MustCompile(`^0\d{9}$`)

func (r *Router) seal(b []byte) ([]byte, error) {
	blk, err := aes.NewCipher(r.key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return g.Seal(nonce, nonce, b, nil), nil
}

func (r *Router) open(b []byte) ([]byte, error) {
	blk, err := aes.NewCipher(r.key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	if len(b) < g.NonceSize() {
		return nil, errors.New("short ciphertext")
	}
	return g.Open(nil, b[:g.NonceSize()], b[g.NonceSize():], nil)
}

// Submit records consent and queues a lead for one campaign.
func (r *Router) Submit(o model.Offer, session string, consent bool, p Payload) (int64, error) {
	if !consent {
		return 0, errors.New("cần bạn đồng ý chia sẻ thông tin với " + o.Partner.Name)
	}
	if o.Campaign.LeadEndpoint == "" {
		return 0, errors.New("chiến dịch này chưa nhận đăng ký qua 3Credit")
	}
	if !o.Campaign.Live(r.Now()) {
		return 0, errors.New("chiến dịch không hoạt động")
	}
	p.Name = strings.TrimSpace(p.Name)
	p.Phone = strings.ReplaceAll(strings.TrimSpace(p.Phone), " ", "")
	if len([]rune(p.Name)) < 2 || len([]rune(p.Name)) > 80 {
		return 0, errors.New("họ tên không hợp lệ")
	}
	if !phoneRe.MatchString(p.Phone) {
		return 0, errors.New("số điện thoại phải gồm 10 chữ số, bắt đầu bằng 0")
	}
	if err := r.capOK(o.Campaign); err != nil {
		return 0, err
	}
	idem := sha256.Sum256([]byte(o.Campaign.ID + "|" + p.Phone + "|" + r.Now().Format("2006-01-02")))
	tx, err := r.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var existing int64
	if err := tx.QueryRow(`SELECT id FROM leads WHERE idempotency_key=?`, hex.EncodeToString(idem[:])).Scan(&existing); err == nil {
		return existing, nil // same person, same campaign, same day
	}
	res, err := tx.Exec(`INSERT INTO consents(session_id,purpose,recipient,text_version) VALUES(?,?,?,?)`,
		session, "share_lead_for_loan_application", o.Partner.ID, ConsentTextVersion)
	if err != nil {
		return 0, err
	}
	cid, _ := res.LastInsertId()
	raw, _ := json.Marshal(p)
	enc, err := r.seal(raw)
	if err != nil {
		return 0, err
	}
	res, err = tx.Exec(`INSERT INTO leads(campaign_id,session_id,consent_id,payload_enc,idempotency_key) VALUES(?,?,?,?,?)`,
		o.Campaign.ID, session, cid, enc, hex.EncodeToString(idem[:]))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, tx.Commit()
}

func (r *Router) capOK(c model.Campaign) error {
	if c.DailyLeadCap > 0 {
		var n int
		r.DB.QueryRow(`SELECT COUNT(*) FROM leads WHERE campaign_id=? AND created_at>=?`, c.ID, r.Now().UTC().Truncate(24*time.Hour)).Scan(&n)
		if n >= c.DailyLeadCap {
			return errors.New("chiến dịch đã đủ số đăng ký hôm nay")
		}
	}
	if c.TotalLeadCap > 0 {
		var n int
		r.DB.QueryRow(`SELECT COUNT(*) FROM leads WHERE campaign_id=?`, c.ID).Scan(&n)
		if n >= c.TotalLeadCap {
			return errors.New("chiến dịch đã đủ số đăng ký")
		}
	}
	return nil
}

// Run processes the queue until ctx is cancelled.
func (r *Router) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := r.ProcessDue(ctx); err != nil {
				log.Printf("lead router: %v", err)
			} else if n > 0 {
				log.Printf("lead router: processed %d", n)
			}
		}
	}
}

type job struct {
	id                  int64
	enc                 []byte
	idem, url, auth, cid string
	attempts            int
}

// ProcessDue sends every queued lead whose next attempt time has passed.
func (r *Router) ProcessDue(ctx context.Context) (int, error) {
	rows, err := r.DB.Query(`SELECT l.id,l.payload_enc,l.idempotency_key,c.lead_endpoint,c.lead_auth_header,c.id,l.attempts
	  FROM leads l JOIN campaigns c ON c.id=l.campaign_id
	  WHERE l.status IN ('queued','retry') AND l.next_attempt_at<=? ORDER BY l.id LIMIT 50`, r.Now().UTC())
	if err != nil {
		return 0, err
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.enc, &j.idem, &j.url, &j.auth, &j.cid, &j.attempts); err != nil {
			rows.Close()
			return 0, err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	for _, j := range jobs {
		r.send(ctx, j)
	}
	return len(jobs), nil
}

func (r *Router) send(ctx context.Context, j job) {
	code, err := r.deliver(ctx, j)
	errTxt := ""
	if err != nil {
		errTxt = err.Error()
	}
	r.DB.Exec(`INSERT INTO lead_attempts(lead_id,http_status,error) VALUES(?,?,?)`, j.id, code, errTxt)
	attempts := j.attempts + 1
	switch {
	case err == nil && code >= 200 && code < 300:
		r.DB.Exec(`UPDATE leads SET status='delivered', attempts=?, last_error='' WHERE id=?`, attempts, j.id)
	case code >= 400 && code < 500 && code != 429:
		r.DB.Exec(`UPDATE leads SET status='failed', attempts=?, last_error=? WHERE id=?`, attempts, fmt.Sprintf("HTTP %d %s", code, errTxt), j.id)
	case attempts >= r.MaxTry:
		r.DB.Exec(`UPDATE leads SET status='failed', attempts=?, last_error=? WHERE id=?`, attempts, fmt.Sprintf("HTTP %d %s", code, errTxt), j.id)
	default:
		backoff := time.Duration(1<<attempts) * time.Minute // 2,4,8,16 min
		r.DB.Exec(`UPDATE leads SET status='retry', attempts=?, next_attempt_at=?, last_error=? WHERE id=?`,
			attempts, r.Now().UTC().Add(backoff), fmt.Sprintf("HTTP %d %s", code, errTxt), j.id)
	}
}

func (r *Router) deliver(ctx context.Context, j job) (int, error) {
	raw, err := r.open(j.enc)
	if err != nil {
		return 0, err
	}
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return 0, err
	}
	body, _ := json.Marshal(map[string]any{
		"campaign_id": j.cid, "name": p.Name, "phone": p.Phone, "loan_amount": p.Amount, "loan_term_months": p.Term,
		"consent_version": ConsentTextVersion,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", j.idem)
	if j.auth != "" {
		if k, v, ok := strings.Cut(j.auth, ":"); ok {
			req.Header.Set(strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	return resp.StatusCode, nil
}

type LeadRow struct {
	ID         int64
	CampaignID string
	Status     string
	Attempts   int
	LastError  string
	CreatedAt  time.Time
	MaskedPhone string
}

// List returns leads for the admin; phone numbers are masked.
func (r *Router) List(limit int) ([]LeadRow, error) {
	rows, err := r.DB.Query(`SELECT id,campaign_id,status,attempts,last_error,created_at,payload_enc FROM leads ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeadRow
	for rows.Next() {
		var l LeadRow
		var enc []byte
		if err := rows.Scan(&l.ID, &l.CampaignID, &l.Status, &l.Attempts, &l.LastError, &l.CreatedAt, &enc); err != nil {
			return nil, err
		}
		if raw, err := r.open(enc); err == nil {
			var p Payload
			if json.Unmarshal(raw, &p) == nil && len(p.Phone) == 10 {
				l.MaskedPhone = p.Phone[:3] + "****" + p.Phone[7:]
			}
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
