CREATE TABLE IF NOT EXISTS users (
  chat_id   INTEGER PRIMARY KEY,
  agreed_at INTEGER
);

CREATE TABLE IF NOT EXISTS monitored_urls (
  id              INTEGER PRIMARY KEY,
  url_encrypted   BLOB    NOT NULL,
  url_hash        TEXT    NOT NULL UNIQUE,
  last_status     INTEGER,
  last_fetched_at INTEGER,
  fail_count      INTEGER NOT NULL DEFAULT 0,
  created_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_monitored_urls_due ON monitored_urls(last_fetched_at);

CREATE TABLE IF NOT EXISTS subscriptions (
  id               INTEGER PRIMARY KEY,
  chat_id          INTEGER NOT NULL,
  monitored_url_id INTEGER NOT NULL REFERENCES monitored_urls(id) ON DELETE CASCADE,
  nickname         TEXT,
  created_at       INTEGER NOT NULL,
  UNIQUE(chat_id, monitored_url_id)
);
CREATE INDEX IF NOT EXISTS idx_subscriptions_chat ON subscriptions(chat_id);

CREATE TABLE IF NOT EXISTS status_history (
  monitored_url_id INTEGER NOT NULL REFERENCES monitored_urls(id) ON DELETE CASCADE,
  status           INTEGER NOT NULL,
  observed_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_status_history_url ON status_history(monitored_url_id, observed_at);
