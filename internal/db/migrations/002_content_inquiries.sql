-- v2: tin tức, cài đặt công ty, yêu cầu tư vấn gửi về 3Credit, yêu cầu xóa dữ liệu.

CREATE TABLE IF NOT EXISTS posts (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  slug          TEXT NOT NULL UNIQUE,
  title         TEXT NOT NULL,
  summary       TEXT NOT NULL DEFAULT '',
  body          TEXT NOT NULL DEFAULT '',
  category      TEXT NOT NULL DEFAULT 'Kiến thức',
  cover_path    TEXT NOT NULL DEFAULT '',
  published     INTEGER NOT NULL DEFAULT 0,
  published_at  TIMESTAMP,
  created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

-- Khách nhờ 3Credit tư vấn. Người nhận dữ liệu là 3Credit, không phải đối tác.
CREATE TABLE IF NOT EXISTS inquiries (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id   TEXT NOT NULL,
  consent_id   INTEGER NOT NULL REFERENCES consents(id),
  payload_enc  BLOB NOT NULL,
  dedupe_key   TEXT NOT NULL UNIQUE,
  source_page  TEXT NOT NULL DEFAULT '',
  campaign_id  TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'new',
  note         TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS deletion_requests (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  phone_hash    TEXT NOT NULL,
  phone_masked  TEXT NOT NULL,
  note          TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'new',
  created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
