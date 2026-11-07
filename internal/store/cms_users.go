package store

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const pbkdf2Iter = 600_000

var usernameRe = regexp.MustCompile(`^[a-z0-9._-]{3,32}$`)

// HashPassword returns "pbkdf2-sha256$iter$salt$hash".
func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	k, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iter, 32)
	if err != nil {
		return "", err
	}
	e := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iter, e.EncodeToString(salt), e.EncodeToString(k)), nil
}

func checkPassword(stored, pw string) bool {
	p := strings.Split(stored, "$")
	if len(p) != 4 || p[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(p[1])
	e := base64.RawStdEncoding
	salt, err1 := e.DecodeString(p[2])
	want, err2 := e.DecodeString(p[3])
	if err != nil || err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

// ValidateNewUser checks username/password rules for CMS accounts.
func ValidateNewUser(user, pw string) error {
	if !usernameRe.MatchString(user) {
		return errors.New("Tên đăng nhập 3–32 ký tự, chỉ gồm chữ thường, số, dấu chấm, gạch dưới, gạch ngang")
	}
	if len([]rune(pw)) < 10 {
		return errors.New("Mật khẩu tối thiểu 10 ký tự")
	}
	return nil
}

func (s *Store) CMSUserCount() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM cms_users`).Scan(&n)
	return n, err
}

func (s *Store) CreateCMSUser(user, pw string) error {
	user = strings.ToLower(strings.TrimSpace(user))
	if err := ValidateNewUser(user, pw); err != nil {
		return err
	}
	h, err := HashPassword(pw)
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(`INSERT INTO cms_users(username,pass_hash) VALUES(?,?)`, user, h); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return errors.New("Tên đăng nhập đã tồn tại")
		}
		return err
	}
	return nil
}

// CreateFirstCMSUser creates the first account only while there is none (atomic).
func (s *Store) CreateFirstCMSUser(user, pw string) error {
	user = strings.ToLower(strings.TrimSpace(user))
	if err := ValidateNewUser(user, pw); err != nil {
		return err
	}
	h, err := HashPassword(pw)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(`INSERT INTO cms_users(username,pass_hash) SELECT ?,? WHERE NOT EXISTS (SELECT 1 FROM cms_users)`, user, h)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("Tài khoản quản trị đã được tạo trước đó, hãy đăng nhập")
	}
	return nil
}

func (s *Store) SetCMSPassword(user, pw string) error {
	if err := ValidateNewUser(user, pw); err != nil {
		return err
	}
	h, err := HashPassword(pw)
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(`UPDATE cms_users SET pass_hash=? WHERE username=?`, h, user); err != nil {
		return err
	}
	_, err = s.DB.Exec(`DELETE FROM cms_sessions WHERE username=?`, user)
	return err
}

func (s *Store) DeleteCMSUser(user string) error {
	_, err := s.DB.Exec(`DELETE FROM cms_users WHERE username=?`, user)
	if err == nil {
		_, err = s.DB.Exec(`DELETE FROM cms_sessions WHERE username=?`, user)
	}
	return err
}

type CMSUser struct {
	Username  string
	CreatedAt time.Time
	LastLogin sql.NullTime
}

func (s *Store) CMSUsers() ([]CMSUser, error) {
	rows, err := s.DB.Query(`SELECT username, created_at, last_login FROM cms_users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CMSUser
	for rows.Next() {
		var u CMSUser
		if err := rows.Scan(&u.Username, &u.CreatedAt, &u.LastLogin); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func tokenHash(tok string) string { h := sha256.Sum256([]byte(tok)); return hex.EncodeToString(h[:]) }

// CMSLogin checks the password and returns a new session token.
func (s *Store) CMSLogin(user, pw string, ttl time.Duration) (string, error) {
	user = strings.ToLower(strings.TrimSpace(user))
	var h string
	err := s.DB.QueryRow(`SELECT pass_hash FROM cms_users WHERE username=?`, user).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		checkPassword("pbkdf2-sha256$600000$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", pw) // equalize timing
		return "", errors.New("bad credentials")
	}
	if err != nil {
		return "", err
	}
	if !checkPassword(h, pw) {
		return "", errors.New("bad credentials")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b)
	if _, err := s.DB.Exec(`INSERT INTO cms_sessions(token_hash,username,expires_at) VALUES(?,?,?)`, tokenHash(tok), user, time.Now().Add(ttl).UTC()); err != nil {
		return "", err
	}
	s.DB.Exec(`UPDATE cms_users SET last_login=CURRENT_TIMESTAMP WHERE username=?`, user)
	s.DB.Exec(`DELETE FROM cms_sessions WHERE expires_at < ?`, time.Now().UTC())
	return tok, nil
}

// CMSSessionUser returns the username for a valid session token, or "".
func (s *Store) CMSSessionUser(tok string) string {
	if len(tok) != 64 {
		return ""
	}
	var u string
	var exp time.Time
	if err := s.DB.QueryRow(`SELECT username, expires_at FROM cms_sessions WHERE token_hash=?`, tokenHash(tok)).Scan(&u, &exp); err != nil {
		return ""
	}
	if time.Now().After(exp) {
		return ""
	}
	return u
}

func (s *Store) CMSLogout(tok string) {
	s.DB.Exec(`DELETE FROM cms_sessions WHERE token_hash=?`, tokenHash(tok))
}
