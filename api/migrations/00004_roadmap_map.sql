-- +goose Up
-- 00004_roadmap_map — bản đồ game (phases/ROADMAP-MAP-IDEA.md §2 + §6).
--
-- Không thêm bảng mới, chỉ 4 cột: stage = 1 bản đồ (terrain + direction),
-- topic = 1 màn (map_x/map_y, NULL = để server layout).
--
-- KHÔNG thêm index mới. Truy vấn bản đồ luôn đi theo path_id → stage_id →
-- position, mà 3 index đó đã có sẵn từ 00001:
--   idx_roadmap_stages_path  (path_id, position, id)
--   idx_roadmap_topics_stage (stage_id, position, id)
--   idx_roadmap_resources_topic (topic_id, position, id)
-- Index theo `terrain`/`direction`/`map_x` sẽ là index chết: không query nào
-- lọc theo chúng (ROADMAP-MAP-IDEA §7.1 chỉ yêu cầu "index nhỏ nếu cần" —
-- ở đây là không cần).

ALTER TABLE roadmap_stages
  ADD COLUMN terrain   TEXT NOT NULL DEFAULT 'meadow'
    CHECK (terrain IN ('meadow', 'desert', 'snow', 'volcano', 'ocean', 'city')),
  ADD COLUMN direction TEXT NOT NULL DEFAULT 'up'
    CHECK (direction IN ('up', 'right'));

ALTER TABLE roadmap_topics
  ADD COLUMN map_x REAL NULL,
  ADD COLUMN map_y REAL NULL;

-- Gán sẵn 10 stage seed theo ROADMAP-MAP-IDEA §6. Lựa chọn hướng đi:
--   zh-g0..zh-g2 = up, zh-g3..zh-g4 = up  → cả path Trung đi dọc, độ khó tăng
--     dần nên giữ 1 hướng để đọc bản đồ như 1 chuỗi bài học liên tục.
--   en-g0/en-g1 = up, en-g2..en-g4 = right → path Anh đổi mùi ở giữa
--     (METHODS → NGOS → NOVEL đổi từ kỹ năng sang đọc), đổi trục sang ngang
--     đánh dấu chỗ bắt đầu học theo sách; M6 đọc `direction` để cuộn đúng chiều.
--
-- UPDATE này chỉ chạy 1 lần lúc migrate, ngay sau khi cột vừa được thêm nên
-- mọi row đang ở đúng giá trị DEFAULT — không ghi đè chỉnh sửa của user. Seed
-- loader (infrastructure/roadmap/seed.go) gán CÙNG bảng này cho stage mới
-- insert qua domain/roadmap.MapDefaults, nên 2 nơi luôn khớp.
UPDATE roadmap_stages SET terrain = 'meadow',  direction = 'up'    WHERE slug = 'zh-g0' AND deleted = 0;
UPDATE roadmap_stages SET terrain = 'meadow',  direction = 'up'    WHERE slug = 'zh-g1' AND deleted = 0;
UPDATE roadmap_stages SET terrain = 'desert',  direction = 'up'    WHERE slug = 'zh-g2' AND deleted = 0;
UPDATE roadmap_stages SET terrain = 'snow',    direction = 'up'    WHERE slug = 'zh-g3' AND deleted = 0;
UPDATE roadmap_stages SET terrain = 'volcano', direction = 'up'    WHERE slug = 'zh-g4' AND deleted = 0;
UPDATE roadmap_stages SET terrain = 'meadow',  direction = 'up'    WHERE slug = 'en-g0' AND deleted = 0;
UPDATE roadmap_stages SET terrain = 'ocean',   direction = 'up'    WHERE slug = 'en-g1' AND deleted = 0;
UPDATE roadmap_stages SET terrain = 'city',    direction = 'right' WHERE slug = 'en-g2' AND deleted = 0;
UPDATE roadmap_stages SET terrain = 'snow',    direction = 'right' WHERE slug = 'en-g3' AND deleted = 0;
UPDATE roadmap_stages SET terrain = 'volcano', direction = 'right' WHERE slug = 'en-g4' AND deleted = 0;

-- +goose Down
ALTER TABLE roadmap_topics
  DROP COLUMN IF EXISTS map_x,
  DROP COLUMN IF EXISTS map_y;
ALTER TABLE roadmap_stages
  DROP COLUMN IF EXISTS direction,
  DROP COLUMN IF EXISTS terrain;
