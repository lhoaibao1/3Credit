-- 3Credit schema v1. Written in portable SQL (SQLite now, PostgreSQL later).

CREATE TABLE IF NOT EXISTS partners (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL,
  legal_name   TEXT NOT NULL DEFAULT '',
  org_type     TEXT NOT NULL DEFAULT '',
  grp          TEXT NOT NULL DEFAULT 'A',
  legal_conf   TEXT NOT NULL DEFAULT '',
  logo_path    TEXT NOT NULL DEFAULT '',
  website      TEXT NOT NULL DEFAULT '',
  integration  TEXT NOT NULL DEFAULT 'pending',
  updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS products (
  id          TEXT PRIMARY KEY,
  partner_id  TEXT NOT NULL REFERENCES partners(id),
  name        TEXT NOT NULL,
  category    TEXT NOT NULL
);

-- Every policy change is a new row. The highest version is current.
CREATE TABLE IF NOT EXISTS policy_versions (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  product_id   TEXT NOT NULL REFERENCES products(id),
  version      INTEGER NOT NULL,
  rules_json   TEXT NOT NULL,
  info_json    TEXT NOT NULL,
  flags_json   TEXT NOT NULL DEFAULT '[]',
  source       TEXT NOT NULL DEFAULT '',
  verified_on  TEXT NOT NULL DEFAULT '',
  changed_by   TEXT NOT NULL DEFAULT '',
  change_note  TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(product_id, version)
);

CREATE TABLE IF NOT EXISTS campaigns (
  id                TEXT PRIMARY KEY,
  product_id        TEXT NOT NULL REFERENCES products(id),
  name              TEXT NOT NULL,
  status            TEXT NOT NULL DEFAULT 'pending',
  priority          INTEGER NOT NULL DEFAULT 0,
  tracking_url      TEXT NOT NULL DEFAULT '',
  subid_param       TEXT NOT NULL DEFAULT 'sub_id',
  postback_secret   TEXT NOT NULL DEFAULT '',
  commission_model  TEXT NOT NULL DEFAULT '',
  commission_value  REAL NOT NULL DEFAULT 0,
  daily_lead_cap    INTEGER NOT NULL DEFAULT 0,
  total_lead_cap    INTEGER NOT NULL DEFAULT 0,
  target_segments   TEXT NOT NULL DEFAULT '[]',
  start_at          TIMESTAMP,
  end_at            TIMESTAMP,
  lead_endpoint     TEXT NOT NULL DEFAULT '',
  lead_auth_header  TEXT NOT NULL DEFAULT '',
  updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS segments (
  id        TEXT PRIMARY KEY,
  name      TEXT NOT NULL,
  rule_json TEXT NOT NULL,
  active    INTEGER NOT NULL DEFAULT 1,
  version   INTEGER NOT NULL DEFAULT 1
);

-- Anonymous search log: profile without PII, for matching analytics.
CREATE TABLE IF NOT EXISTS searches (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id    TEXT NOT NULL,
  profile_json  TEXT NOT NULL,
  segments      TEXT NOT NULL,
  n_eligible    INTEGER NOT NULL,
  n_potential   INTEGER NOT NULL,
  n_unverified  INTEGER NOT NULL,
  n_not         INTEGER NOT NULL,
  top_campaign  TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS clicks (
  click_id     TEXT PRIMARY KEY,
  campaign_id  TEXT NOT NULL REFERENCES campaigns(id),
  partner_id   TEXT NOT NULL,
  product_id   TEXT NOT NULL,
  session_id   TEXT NOT NULL,
  source       TEXT NOT NULL DEFAULT '',
  medium       TEXT NOT NULL DEFAULT '',
  landing      TEXT NOT NULL DEFAULT '',
  device       TEXT NOT NULL DEFAULT '',
  ip_hash      TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_clicks_dedupe ON clicks(session_id, campaign_id, created_at);

CREATE TABLE IF NOT EXISTS conversions (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  network        TEXT NOT NULL,
  conversion_id  TEXT NOT NULL,
  click_id       TEXT NOT NULL REFERENCES clicks(click_id),
  campaign_id    TEXT NOT NULL,
  status         TEXT NOT NULL,
  payout         REAL NOT NULL DEFAULT 0,
  payout_status  TEXT NOT NULL DEFAULT 'unpaid',
  received_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(network, conversion_id)
);

CREATE TABLE IF NOT EXISTS postback_logs (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  network      TEXT NOT NULL,
  query        TEXT NOT NULL,
  signature_ok INTEGER NOT NULL,
  result       TEXT NOT NULL,
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS consents (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id     TEXT NOT NULL,
  purpose        TEXT NOT NULL,
  recipient      TEXT NOT NULL,
  text_version   TEXT NOT NULL,
  granted_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  withdrawn_at   TIMESTAMP
);

-- Lead payload (name, phone) is AES-GCM encrypted at rest.
CREATE TABLE IF NOT EXISTS leads (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  campaign_id      TEXT NOT NULL REFERENCES campaigns(id),
  session_id       TEXT NOT NULL,
  consent_id       INTEGER NOT NULL REFERENCES consents(id),
  payload_enc      BLOB NOT NULL,
  idempotency_key  TEXT NOT NULL UNIQUE,
  status           TEXT NOT NULL DEFAULT 'queued',
  attempts         INTEGER NOT NULL DEFAULT 0,
  next_attempt_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_error       TEXT NOT NULL DEFAULT '',
  created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS lead_attempts (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  lead_id     INTEGER NOT NULL REFERENCES leads(id),
  http_status INTEGER NOT NULL,
  error       TEXT NOT NULL DEFAULT '',
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS audit_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  actor      TEXT NOT NULL,
  action     TEXT NOT NULL,
  entity     TEXT NOT NULL,
  entity_id  TEXT NOT NULL,
  detail     TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
