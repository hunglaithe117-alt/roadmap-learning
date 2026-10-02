-- +goose Up
-- 00003_roadmap_a1 — amendment A1 (plan §5, user duyệt).
--
--   completed_at trên roadmap_stages + roadmap_topics: set khi chuyển sang
--   'done', clear khi rời 'done'. Nuôi biểu đồ tiến độ theo tuần/tháng.
--   deck_id trên roadmap_stages: ON DELETE SET NULL — bấm stage nhảy thẳng
--   /review mà xóa deck không mất stage.
--   is_optional trên roadmap_topics: node "tham khảo" không tính mẫu số %.
--   roadmap_bookmarks: kho link độc lập, đủ 4 cột guid/updated_at/deleted/
--   created_at để sync không bỏ sót.

ALTER TABLE roadmap_stages
  ADD COLUMN completed_at TEXT NULL,
  ADD COLUMN deck_id BIGINT NULL REFERENCES decks (id) ON DELETE SET NULL;
CREATE INDEX idx_roadmap_stages_deck ON roadmap_stages (deck_id) WHERE deck_id IS NOT NULL;
CREATE INDEX idx_roadmap_stages_completed ON roadmap_stages (completed_at) WHERE completed_at IS NOT NULL;

ALTER TABLE roadmap_topics
  ADD COLUMN completed_at TEXT NULL,
  ADD COLUMN is_optional INTEGER NOT NULL DEFAULT 0 CHECK (is_optional IN (0, 1));
CREATE INDEX idx_roadmap_topics_completed ON roadmap_topics (completed_at) WHERE completed_at IS NOT NULL;

CREATE TABLE roadmap_bookmarks (
  id         BIGSERIAL PRIMARY KEY,
  title      TEXT NOT NULL,
  url        TEXT NULL,
  note       TEXT NOT NULL DEFAULT '',
  tags       TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT 'to_read'
               CHECK (status IN ('to_read', 'reading', 'done', 'archived')),
  created_at TEXT NOT NULL,
  guid       TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted    INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX ux_roadmap_bookmarks_guid ON roadmap_bookmarks (guid);
CREATE INDEX idx_roadmap_bookmarks_status ON roadmap_bookmarks (status, id);

-- roadmap_bookmarks là bảng người dùng sửa được → cùng convention v4: trigger
-- chạm updated_at để merge LWW phía kia nhận bản mới nhất.
CREATE TRIGGER trg_roadmap_bookmarks_touch_updated
  BEFORE UPDATE ON roadmap_bookmarks FOR EACH ROW
  EXECUTE FUNCTION langapp_touch_updated_at();

-- +goose Down
DROP TABLE IF EXISTS roadmap_bookmarks CASCADE;
ALTER TABLE roadmap_topics
  DROP COLUMN IF EXISTS is_optional,
  DROP COLUMN IF EXISTS completed_at;
ALTER TABLE roadmap_stages
  DROP COLUMN IF EXISTS deck_id,
  DROP COLUMN IF EXISTS completed_at;
