-- CMS accounts (replaces the single Basic Auth login from .env)
CREATE TABLE IF NOT EXISTS cms_users (
  username   TEXT PRIMARY KEY,
  pass_hash  TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_login TIMESTAMP
);
CREATE TABLE IF NOT EXISTS cms_sessions (
  token_hash TEXT PRIMARY KEY,
  username   TEXT NOT NULL REFERENCES cms_users(username) ON DELETE CASCADE,
  expires_at TIMESTAMP NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
