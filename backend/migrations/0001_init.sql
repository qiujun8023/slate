CREATE EXTENSION IF NOT EXISTS citext;

CREATE TYPE content_kind AS ENUM ('image', 'dynamic');
CREATE TYPE audio_source AS ENUM ('upload', 'tts');
CREATE TYPE audio_status AS ENUM ('pending', 'generating', 'ready', 'failed');

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    email         CITEXT NOT NULL UNIQUE,
    username      CITEXT UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ(3) NOT NULL DEFAULT now()
);

CREATE TABLE groups (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name       VARCHAR(64) NOT NULL,
    sort_order INTEGER NOT NULL,
    created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
    CONSTRAINT groups_owner_sort_key UNIQUE (owner_id, sort_order) DEFERRABLE INITIALLY IMMEDIATE
);

CREATE TABLE devices (
    id            TEXT PRIMARY KEY,
    mac           VARCHAR(17) NOT NULL UNIQUE,
    secret_hash   CHAR(64) NOT NULL UNIQUE,
    pair_code     CHAR(6) NOT NULL UNIQUE,
    name          VARCHAR(64),
    owner_id      TEXT REFERENCES users (id) ON DELETE SET NULL,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    group_id      TEXT REFERENCES groups (id) ON DELETE SET NULL,
    battery_pct   SMALLINT,
    rssi_dbm      SMALLINT,
    fw_version    VARCHAR(32),
    registered_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
    last_seen_at  TIMESTAMPTZ(3),
    created_at    TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
    CONSTRAINT devices_owner_sort_key UNIQUE (owner_id, sort_order) DEFERRABLE INITIALLY IMMEDIATE
);
CREATE INDEX devices_group_id_idx ON devices (group_id);

CREATE TABLE contents (
    id         TEXT PRIMARY KEY,
    group_id   TEXT NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL,
    kind       content_kind NOT NULL,
    frame_name VARCHAR(64),
    image_hash CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
    CONSTRAINT contents_group_sort_key UNIQUE (group_id, sort_order) DEFERRABLE INITIALLY IMMEDIATE
);
CREATE INDEX contents_image_hash_idx ON contents (image_hash);

CREATE TABLE content_dynamic (
    content_id     TEXT PRIMARY KEY REFERENCES contents (id) ON DELETE CASCADE,
    type           VARCHAR(32) NOT NULL,
    config         JSONB NOT NULL,
    data           JSONB,
    rendered_at    TIMESTAMPTZ(3),
    next_run_at    TIMESTAMPTZ(3),
    refresh_due_at TIMESTAMPTZ(3),
    lease_until    TIMESTAMPTZ(3),
    attempts       INTEGER NOT NULL DEFAULT 0,
    last_error     VARCHAR(512)
);
CREATE INDEX content_dynamic_due_idx ON content_dynamic (refresh_due_at) WHERE refresh_due_at IS NOT NULL;

CREATE TABLE content_audio (
    content_id  TEXT PRIMARY KEY REFERENCES contents (id) ON DELETE CASCADE,
    source      audio_source NOT NULL,
    status      audio_status NOT NULL,
    voice       VARCHAR(32),
    text        VARCHAR(512),
    hash        CHAR(64),
    size        INTEGER,
    lease_until TIMESTAMPTZ(3),
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  VARCHAR(512),
    updated_at  TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
    CONSTRAINT content_audio_ready_check CHECK (status <> 'ready' OR (hash IS NOT NULL AND size IS NOT NULL)),
    CONSTRAINT content_audio_tts_check CHECK (source <> 'tts' OR (voice IS NOT NULL AND text IS NOT NULL))
);
CREATE INDEX content_audio_hash_idx ON content_audio (hash) WHERE hash IS NOT NULL;
CREATE INDEX content_audio_tts_queue_idx ON content_audio (updated_at) WHERE source = 'tts' AND status IN ('pending', 'generating');
