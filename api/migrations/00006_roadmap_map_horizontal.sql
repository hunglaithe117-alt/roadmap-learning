-- +goose Up
-- 00006_roadmap_map_horizontal — bản đồ roadmap đi NGANG (roadmap.sh).
--
-- Bản đồ trước đây là dải DỌC. Đo trên app thật (http://localhost:8081):
--   khung map   1377 × 620
--   SVG trong   261 × 1952   (aspect 0.13)
--   viewBox     "369.36 24 261.28 1952"
-- Nguyên nhân gốc ở `domain/roadmap.ComputeLayout`: biên độ 20–70 (đơn vị
-- tuyệt đối) trên trục phụ rộng 1000 ⇒ đường gần như thẳng tuyệt đối.
--
-- Seed có 2 hướng cạnh tranh nhau — bug class "hai nguồn sự thật" đã gặp
-- ~4 lần trong dự án này:
--     direction=right   3 stage  (en-g2 city, en-g3 snow, en-g4 volcano)
--     direction=up      7 stage  (gồm G0 của CẢ 2 path)
-- ⇒ cả 2 path đều mở ra chặng `up`, nên landing view LUÔN là dải dọc hỏng
-- (2 ảnh roadmap-en.png và roadmap-zh.png cùng md5). Không stage `right` nào
-- từng được nhìn bằng mắt.
--
-- Migration này đặt MỌI stage sang `right`. `up` VẪN được `ComputeLayout` hỗ trợ
-- (stage cũ trong DB của user vẫn vẽ được) — chỉ là seed không sinh ra nữa.
-- Nguồn duy nhất cho bảng này là `domain/roadmap.MapDefaults`; migration 00004
-- được sửa song song để DB mới và seed loader không lệch nhau (test
-- `Test_migration_map_table_matches_domain_defaults` khoá 2 nơi đó khớp nhau).
--
-- `map_x`/`map_y` = NULL cho 0 topic tại thời điểm đổi ⇒ không có toạ độ user
-- đặt tay nào cần dọn theo bản đồ cũ.
UPDATE roadmap_stages SET direction = 'right' WHERE deleted = 0;

-- +goose Down
UPDATE roadmap_stages SET direction = 'up' WHERE deleted = 0;
