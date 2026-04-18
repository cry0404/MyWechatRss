package store

const schemaSQL = `
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    email         TEXT    NOT NULL,
    password_hash TEXT    NOT NULL,
    created_at    INTEGER NOT NULL,
    is_admin      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS weread_accounts (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id           INTEGER NOT NULL,
    vid               INTEGER NOT NULL,
    skey_enc          TEXT    NOT NULL,
    refresh_token_enc TEXT    NOT NULL,
    cookies_enc       TEXT    NOT NULL DEFAULT '',
    nickname          TEXT    NOT NULL DEFAULT '',
    avatar            TEXT    NOT NULL DEFAULT '',
    status            TEXT    NOT NULL DEFAULT 'active',
    cooldown_until    INTEGER NOT NULL DEFAULT 0,
    last_ok_at        INTEGER NOT NULL DEFAULT 0,
    last_err          TEXT    NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    device_id         TEXT    NOT NULL,
    install_id        TEXT    NOT NULL,
    device_name       TEXT    NOT NULL DEFAULT '',
    UNIQUE (user_id, vid),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL,
    book_id             TEXT    NOT NULL,
    alias               TEXT    NOT NULL DEFAULT '',
    mp_name             TEXT    NOT NULL DEFAULT '',
    cover_url           TEXT    NOT NULL DEFAULT '',
    fetch_interval_sec  INTEGER NOT NULL DEFAULT 21600,
    fetch_window_start_min INTEGER NOT NULL DEFAULT -1,
    fetch_window_end_min   INTEGER NOT NULL DEFAULT -1,
    last_fetch_at       INTEGER NOT NULL DEFAULT 0,
    last_review_time    INTEGER NOT NULL DEFAULT 0,
    created_at          INTEGER NOT NULL,
    disabled            INTEGER NOT NULL DEFAULT 0,
    UNIQUE (user_id, book_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS articles (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id      TEXT    NOT NULL,
    review_id    TEXT    NOT NULL UNIQUE,
    title        TEXT    NOT NULL DEFAULT '',
    summary      TEXT    NOT NULL DEFAULT '',
    content_html TEXT    NOT NULL DEFAULT '',
    cover_url    TEXT    NOT NULL DEFAULT '',
    url          TEXT    NOT NULL DEFAULT '',
    publish_at   INTEGER NOT NULL DEFAULT 0,
    fetched_at   INTEGER NOT NULL DEFAULT 0,
    read_num     INTEGER NOT NULL DEFAULT 0,
    like_num     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_articles_book_publish
    ON articles(book_id, publish_at DESC);

CREATE TABLE IF NOT EXISTS fetch_logs (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id  INTEGER NOT NULL,
    account_id       INTEGER NOT NULL,
    started_at       INTEGER NOT NULL,
    cost_ms          INTEGER NOT NULL DEFAULT 0,
    new_count        INTEGER NOT NULL DEFAULT 0,
    error            TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS site_config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
