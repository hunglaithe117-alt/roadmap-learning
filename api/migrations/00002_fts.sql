-- +goose Up
-- 00002_fts — thay 2 virtual table FTS5 (dict, en_dict) của SQLite bằng
-- Postgres full-text.
--
-- Hệ quả FTS5 (chấp nhận ở plan §4.2): Postgres không có zhparser sẵn nên
-- tsvector với config 'simple' coi cả chuỗi Hán là MỘT token — tra "你好"
-- bằng @@ không khớp "你好世界". Vì vậy mỗi bảng có 2 đường: tsvector (GIN) cho
-- tiếng Latin/tiếng Việt, pg_trgm + ILIKE làm lưới vớ cho tra Hán 1-N ký tự.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION langapp_dict_vector_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  NEW.search_vector :=
    setweight(to_tsvector('simple', coalesce(NEW.hanzi, '')), 'A') ||
    setweight(to_tsvector('simple', coalesce(NEW.pinyin, '')), 'B') ||
    setweight(to_tsvector('simple', coalesce(NEW.nghia, '')), 'C');
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

ALTER TABLE dict ADD COLUMN search_vector tsvector;
CREATE TRIGGER trg_dict_search_vector
  BEFORE INSERT OR UPDATE ON dict FOR EACH ROW
  EXECUTE FUNCTION langapp_dict_vector_update();

CREATE INDEX idx_dict_search ON dict USING GIN (search_vector);
CREATE INDEX idx_dict_hanzi_trgm ON dict USING GIN (hanzi gin_trgm_ops);
CREATE INDEX idx_dict_pinyin_trgm ON dict USING GIN (pinyin gin_trgm_ops);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION langapp_en_dict_vector_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  NEW.search_vector :=
    setweight(to_tsvector('simple', coalesce(NEW.term, '')), 'A') ||
    setweight(to_tsvector('simple', coalesce(NEW.reading, '')), 'B') ||
    setweight(to_tsvector('simple', coalesce(NEW.gloss, '')), 'C');
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

ALTER TABLE en_dict ADD COLUMN search_vector tsvector;
CREATE TRIGGER trg_en_dict_search_vector
  BEFORE INSERT OR UPDATE ON en_dict FOR EACH ROW
  EXECUTE FUNCTION langapp_en_dict_vector_update();

CREATE INDEX idx_en_dict_search ON en_dict USING GIN (search_vector);
CREATE INDEX idx_en_dict_term_trgm ON en_dict USING GIN (term gin_trgm_ops);
-- Tra chính xác không phân biệt hoa/thường: api/english.go:274 (v1) dùng
-- `WHERE term = ? COLLATE NOCASE` — CÚ PHÁP KHÔNG TỒN TẠI ở Postgres (lỗi
-- syntax). M3 phải port thành `lower(term) = lower(?)`; index này phục vụ
-- đúng câu đó. Không sửa api/english.go ở M1 vì app cũ vẫn chạy SQLite.
CREATE INDEX idx_en_dict_term_lower ON en_dict (lower(term));

-- dict_search: tsquery (websearch_to_tsquery chịu được cú pháp người dùng gõ
-- thẳng, không ném lỗi syntax như to_tsquery) OR ILIKE '%q%' (lưới vớ CJK
-- ngắn). ILIKE đặt sau @@ trong OR nhưng Postgres không bảo đảm thứ tự
-- short-circuit — index GIN vẫn được dùng cho nhánh @@ của query plan.
--
-- `q` là input người dùng, nên phải khử ký tự wildcard của LIKE trước:
-- nội suy thô `%` sẽ thành `ILIKE '%%%'` → khớp mọi row, còn q chỉ toàn dấu
-- câu thì cũng trả nguyên bảng. `\\` vì escape mặc định của LIKE là `\`.
--
-- Default lim = 10 khớp LIMIT của caller v1 (main.go:36 tra 10 kết quả);
-- dict_search cũ từng default 20 là lệch.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION dict_search(q text, lim int DEFAULT 10)
RETURNS TABLE (hanzi text, pinyin text, nghia text)
LANGUAGE plpgsql STABLE AS $$
DECLARE
  qc text;
BEGIN
  qc := regexp_replace(q, '[%_\\]', '', 'g');
  IF qc IS NULL OR btrim(qc) = '' THEN
    RETURN;
  END IF;
  RETURN QUERY
    SELECT d.hanzi, d.pinyin, d.nghia
    FROM dict d
    WHERE d.search_vector @@ websearch_to_tsquery('simple', qc)
       OR d.hanzi ILIKE '%' || qc || '%'
       OR d.pinyin ILIKE '%' || qc || '%'
    ORDER BY (d.hanzi ILIKE '%' || qc || '%') DESC, d.hanzi
    LIMIT GREATEST(lim, 1);
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION en_dict_search(q text, lim int DEFAULT 10)
RETURNS TABLE (lang text, term text, reading text, gloss text)
LANGUAGE plpgsql STABLE AS $$
DECLARE
  qc text;
BEGIN
  qc := regexp_replace(q, '[%_\\]', '', 'g');
  IF qc IS NULL OR btrim(qc) = '' THEN
    RETURN;
  END IF;
  RETURN QUERY
    SELECT e.lang, e.term, e.reading, e.gloss
    FROM en_dict e
    WHERE e.search_vector @@ websearch_to_tsquery('simple', qc)
       OR e.term ILIKE '%' || qc || '%'
    ORDER BY (e.term ILIKE '%' || qc || '%') DESC, e.term
    LIMIT GREATEST(lim, 1);
END;
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS en_dict_search(text, int);
DROP FUNCTION IF EXISTS dict_search(text, int);
DROP TRIGGER IF EXISTS trg_en_dict_search_vector ON en_dict;
DROP FUNCTION IF EXISTS langapp_en_dict_vector_update();
DROP INDEX IF EXISTS idx_en_dict_term_lower;
DROP INDEX IF EXISTS idx_en_dict_term_trgm;
DROP INDEX IF EXISTS idx_en_dict_search;
DROP TRIGGER IF EXISTS trg_dict_search_vector ON dict;
DROP FUNCTION IF EXISTS langapp_dict_vector_update();
DROP INDEX IF EXISTS idx_dict_pinyin_trgm;
DROP INDEX IF EXISTS idx_dict_hanzi_trgm;
DROP INDEX IF EXISTS idx_dict_search;
ALTER TABLE en_dict DROP COLUMN IF EXISTS search_vector;
ALTER TABLE dict DROP COLUMN IF EXISTS search_vector;
DROP EXTENSION IF EXISTS pg_trgm;
