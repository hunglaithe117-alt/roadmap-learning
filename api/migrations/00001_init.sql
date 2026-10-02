-- +goose Up
-- 00001_init — dịch api/schema.sql (SQLite v4) sang PostgreSQL.
--
-- Nguồn: api/schema.sql. Giữ nguyên tên cột, ngữ nghĩa, FK cascade, index,
-- CHECK và trigger chạm updated_at. Ba khác biệt bắt buộc khi đổi engine:
--   1. INTEGER PRIMARY KEY AUTOINCREMENT -> BIGSERIAL PRIMARY KEY.
--   2. Trigger chạm updated_at: `NEW.updated_at IS OLD.updated_at` không tồn
--      tại ở Postgres -> `IS NOT DISTINCT FROM`; strftime -> to_char(now()).
--      Chuyển từ AFTER UPDATE (UPDATE lồng) sang BEFORE UPDATE (gán NEW) nên
--      đệ quy vô hạn của SQLite không còn; điều kiện "OLD khác now" cũng
--      thành thừa vì gán trùng là no-op.
--   3. Guard lang của decks (SQLite không ALTER được CHECK nên dùng BEFORE
--      trigger) thành CHECK constraint — Postgres thêm CHECK được.

CREATE TABLE schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);

CREATE TABLE decks (
  id         BIGSERIAL PRIMARY KEY,
  name       TEXT NOT NULL,
  lang       TEXT NOT NULL DEFAULT 'zh' CHECK (lang IN ('zh', 'en')),
  created_at TEXT NOT NULL,
  guid       TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE cards (
  id         BIGSERIAL PRIMARY KEY,
  deck_id    BIGINT NOT NULL REFERENCES decks (id) ON DELETE CASCADE,
  front      TEXT NOT NULL,
  back       TEXT NOT NULL,
  pinyin     TEXT NOT NULL DEFAULT '',
  due_at     TEXT NOT NULL,
  stability  DOUBLE PRECISION NOT NULL DEFAULT 0,
  difficulty DOUBLE PRECISION NOT NULL DEFAULT 0,
  reps       INTEGER NOT NULL DEFAULT 0,
  lapses     INTEGER NOT NULL DEFAULT 0,
  state      TEXT NOT NULL DEFAULT 'new',
  created_at TEXT NOT NULL,
  tone       TEXT NULL,
  ipa        TEXT NULL,
  stress     TEXT NULL,
  audio_url  TEXT NULL,
  guid       TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_cards_deck_due ON cards (deck_id, due_at);
-- Blocker #2 (v2 gate): partial index chỉ cover deleted=0 — xóa mềm rồi tạo
-- lại cùng front được REUSE, sync peer re-create không bị nuốt.
CREATE UNIQUE INDEX ux_cards_deck_front ON cards (deck_id, front) WHERE deleted = 0;
CREATE UNIQUE INDEX ux_decks_guid ON decks (guid);
CREATE UNIQUE INDEX ux_cards_guid ON cards (guid);

CREATE TABLE reviews (
  id          BIGSERIAL PRIMARY KEY,
  card_id     BIGINT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
  -- Thang 1-4 là hợp đồng đóng băng với UI (v1 ReviewHandler từ chối ngoài
  -- 1-4; xem internal/domain/srs Grade). CHECK ở DB như decks.lang: chặn
  -- sớm ở tầng dưới thay vì để lọt xuống tính toán SRS.
  grade       INTEGER NOT NULL CHECK (grade BETWEEN 1 AND 4),
  reviewed_at TEXT NOT NULL,
  next_due_at TEXT NOT NULL,
  guid        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_reviews_card ON reviews (card_id);
CREATE UNIQUE INDEX ux_reviews_guid ON reviews (guid);

CREATE TABLE notes (
  id         BIGSERIAL PRIMARY KEY,
  card_id    BIGINT REFERENCES cards (id) ON DELETE CASCADE,
  text       TEXT NOT NULL,
  created_at TEXT NOT NULL,
  guid       TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX ux_notes_guid ON notes (guid);

-- dict / en_dict: SQLite là virtual table FTS5, Postgres không có tương đương
-- 1:1 (virtual table không nhận index thật). Ở đây tạo bảng thật giữ đúng
-- tên cột; 00002_fts.sql gắn tsvector + GIN + trigger search.
CREATE TABLE dict (
  id     BIGSERIAL PRIMARY KEY,
  hanzi  TEXT NOT NULL DEFAULT '',
  pinyin TEXT NOT NULL DEFAULT '',
  nghia  TEXT NOT NULL DEFAULT ''
);
CREATE TABLE en_dict (
  id      BIGSERIAL PRIMARY KEY,
  lang    TEXT NOT NULL DEFAULT 'en',
  term    TEXT NOT NULL DEFAULT '',
  reading TEXT NOT NULL DEFAULT '',
  gloss   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE sync_meta (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL
);
CREATE TABLE sync_conflicts (
  id         BIGSERIAL PRIMARY KEY,
  table_name TEXT NOT NULL,
  guid       TEXT NOT NULL DEFAULT '',
  winner     TEXT NOT NULL DEFAULT '',
  detail     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX idx_sync_conflicts_table ON sync_conflicts (table_name);

-- ── Roadmap / learning path (R1a) ──────────────────────────────────────────
-- Cây 5 tầng: paths > stages > {milestones, topics > resources}.
-- Bám convention v4: mọi bảng người dùng sửa được đều có guid UNIQUE,
-- updated_at, deleted + trigger chạm updated_at + xóa mềm (sync dựa vào đó).
-- status ∈ not_started|in_progress|done|skipped (CHECK ở DB + validate ở API).

CREATE TABLE roadmap_paths (
  id          BIGSERIAL PRIMARY KEY,
  slug        TEXT NOT NULL,
  language    TEXT NOT NULL DEFAULT 'zh',
  title       TEXT NOT NULL,
  overview    TEXT NOT NULL DEFAULT '',
  is_builtin  INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL,
  guid        TEXT NOT NULL DEFAULT '',
  updated_at  TEXT NOT NULL DEFAULT '',
  deleted     INTEGER NOT NULL DEFAULT 0
);
-- slug là natural key của seed (chỉ insert khi slug chưa có) → UNIQUE để
-- tạo trùng trả 409. Tombstone vẫn giữ slug: xoá mềm rồi tạo lại cùng slug
-- → 409 (user đổi slug, không phải "hồi sinh" như cards).
CREATE UNIQUE INDEX ux_roadmap_paths_slug ON roadmap_paths (slug);
CREATE UNIQUE INDEX ux_roadmap_paths_guid ON roadmap_paths (guid);

CREATE TABLE roadmap_stages (
  id              BIGSERIAL PRIMARY KEY,
  path_id         BIGINT NOT NULL REFERENCES roadmap_paths (id) ON DELETE CASCADE,
  slug            TEXT NOT NULL,
  title           TEXT NOT NULL,
  goal            TEXT NOT NULL DEFAULT '',
  position        INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
  duration_weeks  INTEGER NOT NULL DEFAULT 0 CHECK (duration_weeks >= 0),
  status          TEXT NOT NULL DEFAULT 'not_started'
                    CHECK (status IN ('not_started', 'in_progress', 'done', 'skipped')),
  status_note     TEXT NOT NULL DEFAULT '',
  created_at      TEXT NOT NULL,
  guid            TEXT NOT NULL DEFAULT '',
  updated_at      TEXT NOT NULL DEFAULT '',
  deleted         INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX ux_roadmap_stages_path_slug ON roadmap_stages (path_id, slug);
CREATE UNIQUE INDEX ux_roadmap_stages_guid ON roadmap_stages (guid);
CREATE INDEX idx_roadmap_stages_path ON roadmap_stages (path_id, position, id);

CREATE TABLE roadmap_milestones (
  id          BIGSERIAL PRIMARY KEY,
  stage_id    BIGINT NOT NULL REFERENCES roadmap_stages (id) ON DELETE CASCADE,
  text        TEXT NOT NULL,
  position    INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
  created_at  TEXT NOT NULL,
  guid        TEXT NOT NULL DEFAULT '',
  updated_at  TEXT NOT NULL DEFAULT '',
  deleted     INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX ux_roadmap_milestones_guid ON roadmap_milestones (guid);
CREATE INDEX idx_roadmap_milestones_stage ON roadmap_milestones (stage_id, position, id);

CREATE TABLE roadmap_topics (
  id           BIGSERIAL PRIMARY KEY,
  stage_id     BIGINT NOT NULL REFERENCES roadmap_stages (id) ON DELETE CASCADE,
  title        TEXT NOT NULL,
  why          TEXT NOT NULL DEFAULT '',
  -- activities: JSON array string (json.Marshal []string) — docs seed có
  -- nhiều hoạt động/tài liệu mỗi topic, gộp vào `why` sẽ mất cấu trúc.
  activities   TEXT NOT NULL DEFAULT '',
  position     INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
  status       TEXT NOT NULL DEFAULT 'not_started'
                  CHECK (status IN ('not_started', 'in_progress', 'done', 'skipped')),
  status_note  TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL,
  guid         TEXT NOT NULL DEFAULT '',
  updated_at   TEXT NOT NULL DEFAULT '',
  deleted      INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX ux_roadmap_topics_guid ON roadmap_topics (guid);
CREATE INDEX idx_roadmap_topics_stage ON roadmap_topics (stage_id, position, id);

CREATE TABLE roadmap_resources (
  id          BIGSERIAL PRIMARY KEY,
  topic_id    BIGINT NOT NULL REFERENCES roadmap_topics (id) ON DELETE CASCADE,
  title       TEXT NOT NULL,
  url         TEXT NULL,
  kind        TEXT NOT NULL DEFAULT '',
  note        TEXT NOT NULL DEFAULT '',
  position    INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
  created_at  TEXT NOT NULL,
  guid        TEXT NOT NULL DEFAULT '',
  updated_at  TEXT NOT NULL DEFAULT '',
  deleted     INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX ux_roadmap_resources_guid ON roadmap_resources (guid);
CREATE INDEX idx_roadmap_resources_topic ON roadmap_resources (topic_id, position, id);

-- Chạm updated_at khi UPDATE không set tường minh (merge set tường minh nên
-- NEW != OLD → trigger bỏ qua). Soft-delete, đổi status, đổi tên đều đổi mốc
-- → merge LWW phía kia nhận bản mới nhất.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION langapp_touch_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.updated_at IS NOT DISTINCT FROM OLD.updated_at THEN
    NEW.updated_at := to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"');
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_decks_touch_updated
  BEFORE UPDATE ON decks FOR EACH ROW EXECUTE FUNCTION langapp_touch_updated_at();
CREATE TRIGGER trg_cards_touch_updated
  BEFORE UPDATE ON cards FOR EACH ROW EXECUTE FUNCTION langapp_touch_updated_at();
CREATE TRIGGER trg_roadmap_paths_touch_updated
  BEFORE UPDATE ON roadmap_paths FOR EACH ROW EXECUTE FUNCTION langapp_touch_updated_at();
CREATE TRIGGER trg_roadmap_stages_touch_updated
  BEFORE UPDATE ON roadmap_stages FOR EACH ROW EXECUTE FUNCTION langapp_touch_updated_at();
CREATE TRIGGER trg_roadmap_milestones_touch_updated
  BEFORE UPDATE ON roadmap_milestones FOR EACH ROW EXECUTE FUNCTION langapp_touch_updated_at();
CREATE TRIGGER trg_roadmap_topics_touch_updated
  BEFORE UPDATE ON roadmap_topics FOR EACH ROW EXECUTE FUNCTION langapp_touch_updated_at();
CREATE TRIGGER trg_roadmap_resources_touch_updated
  BEFORE UPDATE ON roadmap_resources FOR EACH ROW EXECUTE FUNCTION langapp_touch_updated_at();

-- schema_migrations là bảng version của app v1 (sync.go/backup.go đọc
-- `MAX(version)` để chặn merge giữa 2 máy lệch schema, v1 db.go:77 insert
-- version 4). Goose có bảng version riêng (`goose_db_version`) nhưng app v1
-- không đọc nó, nên bảng này sẽ rỗng nếu không seed -> `MAX(version)` = NULL
-- và `CheckVersion` pass vacuously. Giữ đúng số 4 của schema.sql.
INSERT INTO schema_migrations (version, applied_at)
VALUES (4, to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'))
ON CONFLICT (version) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS roadmap_resources CASCADE;
DROP TABLE IF EXISTS roadmap_topics CASCADE;
DROP TABLE IF EXISTS roadmap_milestones CASCADE;
DROP TABLE IF EXISTS roadmap_stages CASCADE;
DROP TABLE IF EXISTS roadmap_paths CASCADE;
DROP TABLE IF EXISTS sync_conflicts CASCADE;
DROP TABLE IF EXISTS sync_meta CASCADE;
DROP TABLE IF EXISTS en_dict CASCADE;
DROP TABLE IF EXISTS dict CASCADE;
DROP TABLE IF EXISTS notes CASCADE;
DROP TABLE IF EXISTS reviews CASCADE;
DROP TABLE IF EXISTS cards CASCADE;
DROP TABLE IF EXISTS decks CASCADE;
DROP FUNCTION IF EXISTS langapp_touch_updated_at();
DROP TABLE IF EXISTS schema_migrations CASCADE;
