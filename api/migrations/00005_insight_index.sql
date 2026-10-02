-- +goose Up
-- 00005_insight_index — index phục vụ truy vấn của bounded context `insight`
-- (dashboard + sổ lỗi). KHÔNG thêm cột mới, chỉ thêm index.
--
-- Cả 3 index dưới đây đều có 1 truy vấn cụ thể gọi tới; ghi rõ ở comment từng
-- cái để index chết không lọt vào đây (tiền lý M2 đã dọn 1 loạt index theo
-- metadata không ai lọc).

-- 1) `Stats(range)` chạy `SELECT COUNT(*), SUM(grade >= 3) FROM reviews
--    WHERE reviewed_at >= ?` mỗi lần mở dashboard. Không có index nào trên
--    `reviewed_at` (chỉ có `idx_reviews_card` trên `card_id`), nên bảng `reviews`
--    — bảng CHỈ TĂNG, và là bảng phình nhanh nhất app — bị seq scan mỗi lần.
--    Đây là index có lý do rõ nhất trong migration này.
CREATE INDEX idx_reviews_reviewed_at ON reviews (reviewed_at);

-- 2) `ComputeStreak` đọc `SELECT DISTINCT substr(reviewed_at, 1, 10) ... ORDER BY
--    d DESC`. Expression index trên đúng biểu thức đó biến "quét toàn bộ lịch
--    sử ôn rồi distinct + sort" thành "index-only scan". Cùng lý do NULL-safety
--    với 00001: cột là TEXT RFC3339 nên phải cắt chuỗi, không dùng được
--    `::date` (sẽ phá index khi sau này thêm index theo ngày).
CREATE INDEX idx_reviews_reviewed_day ON reviews (substr(reviewed_at, 1, 10));

-- 3) Sổ lỗi: `ListErrorNotes` / `insight.ListErrorNotes` lọc
--    `text LIKE 'ERR|%' AND text NOT LIKE 'THIEU|%' ORDER BY id DESC`.
--    Partial index theo ĐÚNG predicate đó: Postgres chỉ dùng được khi predicate
--    của index được query chứa trong WHERE. Index gộp `(card_id, id DESC)` phục
--    luôn đường lọc theo 1 thẻ (dùng cho `GET /api/errors?card_id=`).
--
--    KHÔNG index `text` bằng `pg_trgm` ở đây: prefix cố định 3 ký tự đã khớp
--    btree của partial index, trigram chỉ hữu ích khi tìm chuỗi con bất kỳ.
CREATE INDEX idx_notes_err ON notes (id DESC) WHERE text LIKE 'ERR|%';
CREATE INDEX idx_notes_err_card ON notes (card_id, id DESC) WHERE text LIKE 'ERR|%';

-- +goose Down
DROP INDEX IF EXISTS idx_notes_err_card;
DROP INDEX IF EXISTS idx_notes_err;
DROP INDEX IF EXISTS idx_reviews_reviewed_day;
DROP INDEX IF EXISTS idx_reviews_reviewed_at;
