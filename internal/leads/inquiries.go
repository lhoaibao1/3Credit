package leads

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Inquiry: a customer asks 3Credit itself to call back with advice.
// The data recipient is 3Credit; nothing is sent to a partner from here.

const InquiryConsentVersion = "inquiry-consent-v1-2026-11"

type InquiryInput struct {
	Name       string  `json:"name"`
	Phone      string  `json:"phone"`
	Need       string  `json:"need"`
	Amount     float64 `json:"amount"`
	Term       int     `json:"term"`
	Job        string  `json:"job"`
	Income     float64 `json:"income"`
	Region     string  `json:"region"`
	Credit     string  `json:"credit"`
	SocialIns  string  `json:"social_ins,omitempty"`
	LifeIns    string  `json:"life_ins,omitempty"`
	Message    string  `json:"message"`
	CampaignID string  `json:"campaign_id"`
	SourcePage string  `json:"source_page"`
}

var InquiryStatuses = []string{"new", "contacted", "qualified", "sent_to_partner", "closed"}

// SubmitInquiry stores a consented callback request (encrypted). Same phone
// on the same day is deduplicated.
func (r *Router) SubmitInquiry(session string, consent bool, in InquiryInput) (int64, bool, error) {
	if !consent {
		return 0, false, errors.New("cần bạn đồng ý để 3Credit liên hệ tư vấn")
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Phone = strings.ReplaceAll(strings.TrimSpace(in.Phone), " ", "")
	in.Phone = strings.ReplaceAll(in.Phone, ".", "")
	if strings.HasPrefix(in.Phone, "+84") {
		in.Phone = "0" + in.Phone[3:]
	}
	if n := len([]rune(in.Name)); n < 2 || n > 80 {
		return 0, false, errors.New("vui lòng nhập họ tên")
	}
	if !phoneRe.MatchString(in.Phone) {
		return 0, false, errors.New("số điện thoại phải gồm 10 chữ số, bắt đầu bằng 0")
	}
	if len([]rune(in.Message)) > 500 {
		in.Message = string([]rune(in.Message)[:500])
	}
	key := sha256.Sum256([]byte("inq|" + in.Phone + "|" + r.Now().Format("2006-01-02")))
	tx, err := r.DB.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	var existing int64
	if err := tx.QueryRow(`SELECT id FROM inquiries WHERE dedupe_key=?`, hex.EncodeToString(key[:])).Scan(&existing); err == nil {
		return existing, true, nil
	}
	res, err := tx.Exec(`INSERT INTO consents(session_id,purpose,recipient,text_version) VALUES(?,?,?,?)`,
		session, "callback_financial_advice", "3credit", InquiryConsentVersion)
	if err != nil {
		return 0, false, err
	}
	cid, _ := res.LastInsertId()
	raw, _ := json.Marshal(in)
	enc, err := r.seal(raw)
	if err != nil {
		return 0, false, err
	}
	res, err = tx.Exec(`INSERT INTO inquiries(session_id,consent_id,payload_enc,dedupe_key,source_page,campaign_id) VALUES(?,?,?,?,?,?)`,
		session, cid, enc, hex.EncodeToString(key[:]), trunc(in.SourcePage, 120), trunc(in.CampaignID, 40))
	if err != nil {
		return 0, false, err
	}
	id, _ := res.LastInsertId()
	return id, false, tx.Commit()
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

type Inquiry struct {
	ID        int64
	Status    string
	Note      string
	Source    string
	Campaign  string
	CreatedAt time.Time
	In        InquiryInput
}

// Inquiries returns decrypted inquiries for authorised admins.
func (r *Router) Inquiries(status string, limit int) ([]Inquiry, error) {
	q := `SELECT id,status,note,source_page,campaign_id,created_at,payload_enc FROM inquiries`
	args := []any{}
	if status != "" {
		q += ` WHERE status=?`
		args = append(args, status)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Inquiry
	for rows.Next() {
		var i Inquiry
		var enc []byte
		if err := rows.Scan(&i.ID, &i.Status, &i.Note, &i.Source, &i.Campaign, &i.CreatedAt, &enc); err != nil {
			return nil, err
		}
		if raw, err := r.open(enc); err == nil {
			json.Unmarshal(raw, &i.In)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *Router) UpdateInquiry(id int64, status, note string) error {
	ok := false
	for _, s := range InquiryStatuses {
		if s == status {
			ok = true
		}
	}
	if !ok {
		return errors.New("trạng thái không hợp lệ")
	}
	if len([]rune(note)) > 1000 {
		note = string([]rune(note)[:1000])
	}
	_, err := r.DB.Exec(`UPDATE inquiries SET status=?, note=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, note, id)
	return err
}

// ExportCSV writes inquiries as CSV (UTF-8 with BOM so Excel shows Vietnamese).
func (r *Router) ExportCSV(w io.Writer, status string, label func(string) string) error {
	list, err := r.Inquiries(status, 100000)
	if err != nil {
		return err
	}
	w.Write([]byte("\xEF\xBB\xBF"))
	cw := csv.NewWriter(w)
	cw.Write([]string{"ID", "Thời gian", "Họ tên", "Số điện thoại", "Nhu cầu", "Số tiền (triệu)", "Kỳ hạn (tháng)", "Công việc", "Thu nhập (triệu)", "Khu vực", "Lịch sử tín dụng (tự khai)", "BHXH", "BHNT", "Lời nhắn", "Sản phẩm quan tâm", "Trang", "Trạng thái", "Ghi chú"})
	for _, i := range list {
		cw.Write([]string{
			fmt.Sprint(i.ID), i.CreatedAt.Local().Format("02/01/2006 15:04"), safeCell(i.In.Name), "'" + i.In.Phone, label(i.In.Need),
			fmt.Sprintf("%.0f", i.In.Amount/1e6), fmt.Sprint(i.In.Term), label(i.In.Job), fmt.Sprintf("%g", i.In.Income), label(i.In.Region), label(i.In.Credit), label(i.In.SocialIns), label(i.In.LifeIns),
			safeCell(i.In.Message), i.Campaign, i.Source, i.Status, safeCell(i.Note),
		})
	}
	cw.Flush()
	return cw.Error()
}

// safeCell prevents CSV formula injection in spreadsheet apps.
func safeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// DeletionRequest records a "delete my data" request (phone stored hashed + masked).
func (r *Router) DeletionRequest(phone, note string) error {
	phone = strings.ReplaceAll(strings.TrimSpace(phone), " ", "")
	if !phoneRe.MatchString(phone) {
		return errors.New("số điện thoại phải gồm 10 chữ số, bắt đầu bằng 0")
	}
	h := sha256.Sum256(append(r.key, []byte(phone)...))
	if len([]rune(note)) > 500 {
		note = string([]rune(note)[:500])
	}
	_, err := r.DB.Exec(`INSERT INTO deletion_requests(phone_hash,phone_masked,note) VALUES(?,?,?)`,
		hex.EncodeToString(h[:]), phone[:3]+"****"+phone[7:], note)
	return err
}

// MatchesPhone reports whether a deletion request hash belongs to a phone.
func (r *Router) PhoneHash(phone string) string {
	h := sha256.Sum256(append(r.key, []byte(phone)...))
	return hex.EncodeToString(h[:])
}

// EraseByPhone deletes inquiries and leads whose decrypted phone matches.
func (r *Router) EraseByPhone(hash string) (int, error) {
	n := 0
	for _, t := range []string{"inquiries", "leads"} {
		rows, err := r.DB.Query(`SELECT id,payload_enc FROM ` + t)
		if err != nil {
			return n, err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			var enc []byte
			rows.Scan(&id, &enc)
			raw, err := r.open(enc)
			if err != nil {
				continue
			}
			var p struct {
				Phone string `json:"phone"`
			}
			json.Unmarshal(raw, &p)
			if p.Phone != "" && r.PhoneHash(p.Phone) == hash {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, id := range ids {
			if t == "leads" {
				r.DB.Exec(`DELETE FROM lead_attempts WHERE lead_id=?`, id)
			}
			if _, err := r.DB.Exec(`DELETE FROM `+t+` WHERE id=?`, id); err == nil {
				n++
			}
		}
	}
	return n, nil
}
