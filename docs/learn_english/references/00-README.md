# References — kho nội dung fetch/build cho plan VuEnglish

> Thư mục này chứa TOÀN BỘ nội dung fetch thô + research nền dùng để viết bộ plan
> trong `docs/`. Đọc kèm quy ước nhãn ở `docs/00-gioi-han-du-lieu-va-gia-dinh.md`.

## Phân loại

- **File raw gốc (đọc đầu tiên)** — HTML nguyên bản tải trực tiếp, chưa tóm tắt:
  `raw/` (5 files + `MANIFEST.md`). Mở bằng trình duyệt để xem đầy đủ.
- **Trích nguồn VuEnglish ([Nguồn])** — nội dung crawl trực tiếp từ 5 links gốc:
  - `10-crawl-batch-a.md` — THIEU 2023 (38 sessions) + TAP I 2024 (24 sessions) + NGONG registration 2026.
  - `11-crawl-batch-b.md` — NGONG syllabus 2026 (S01–S24) + Knowledge-Sharing index (21+1).
  - `12-crawl-chi-tiet-5-links.md` — (đang crawl bổ sung, lib-5) chi tiết từng buổi + sub-pages.
- **Kiến thức nền chung (KHÔNG phải lời VuEnglish)** — research bổ trợ để giải thích
  chi tiết, khi đưa vào `docs/` phải gắn nhãn `[Tham khảo thêm]`:
  - `20-background-nghe-noi.md` — 12 kỹ thuật phát âm/nối-nuốt + shadowing + 14 links + shows.
  - `21-background-tap-thieu-sharing.md` — TMRND/PVO/SRS + 11 mảng viết + 20 links + 8 chủ đề.
  - `22-link-status.md` — trạng thái sống/chết 39 links ngoài (check 2026-09-23).

## Ngày fetch

- Batch A/B (lib-1/lib-2): 2026-09-23, cả 5 URLs status 200.
- Background NGONG/TAP-THIEU (lib-3/lib-4): 2026-09-23.
- Crawl chi tiết (lib-5): đang chạy.

## Lưu ý

- 2 syllabus tổng thể THIEU/TAP dạng ảnh — webfetch không OCR được (ghi nhận trong file 10).
- Links tài liệu ngoài do librarian "tin là tồn tại", chưa verify sống — kiểm tra lại
  trước khi dùng (xem Gate 1 expansion, guardrail F5).

## Hiệu đính sau audit (2026-09-23)

Chuẩn đối chiếu: `raw/*.html` (curl 2026-09-23, HTTP 200). Chỉ bổ sung/sửa đúng mục audit flag, không viết lại nội dung cũ. Chi tiết xem mục `Bổ sung audit 2026-09-23` trong từng file.

- `10-crawl-batch-a.md` (THIỂU, raw/01): thêm S13 Homework 2 điền linking words (40 câu); S16 measurement modifiers + dialect + PURPOSE∈CAUSE/EFFECT, EXCEPTION∈EXCLUSION; S24 warm-up quảng cáo/rao vặt lỗi linking scope; S32 spoken qualifying structures + qualification văn luật/khi nói; tách cụm 34-37 thành S34 Engagement lead-in (Martin) / S35 chữa Engagement + Toulmin / S36 Issue essays TOEFL-IELTS vs GRE-LSAT-GMAT + Idea Map/Peer Review / S37 Fallacy part 2 + scandal từ thiện VN + homework GRE; S38 multimedia bổ sung + bản dịch/công chứng tài liệu Việt.
- `10-crawl-batch-a.md` (TẠP I, raw/02): chuyển S23 (`The Locked Room` + hoàn thành `Bảo Hiểm Thất Nghiệp` + câu văn học VN Final Review) ra khỏi nhóm Sociology sang nhóm HUMOR (sửa 2 bullets, giữ nội dung cũ); S24 bổ sung Intrinsic vs Extrinsic aging + routine Cleansing...Tech-based + 3 tên skincare phụ grep thấy (`Exactly how your skin changes in your 40s, 50s, and 60s` / `Does drinking collagen...` / `How fake science sells wellness`), 7 bài còn lại ghi `không trích được từ raw`.
- `10-crawl-batch-a.md` + `11-crawl-batch-b.md` (Registration, raw/03): thêm link `vuenglish-faq.html` (5 lần trong raw/03, sidebar); ghi tần suất `registration-main-classes.html` **6 lần**; ghi 2 câu grep thấy (`địa chỉ này khác với địa chỉ email thường dùng để gửi các thông báo chung về mở đăng ký`, `chỉ nên gửi đăng ký và lịch học tự chọn khi đã sẵn sàng về tài chính`); bỏ qua các cụm không grep thấy (`địa chỉ duy nhất`, `từ mail nhận bài giảng`, cụm nguyên văn `chỉ nên gửi khi sẵn sàng tài chính` không xuất hiện).
- `11-crawl-batch-b.md` (Sharing, raw/05): thêm full 22 slugs + tháng 06/07/08 (18×06, 3×07, 1×08), ngày index `June 15, 2026`; phân biệt `synonym.html` vs `synonym_01833627631.html`, `ielts-test-taker-notes.html` (06) vs `knowledge-sharing-ielts.html` (07), `vu-fitness-earlier-better.html`; thêm câu `tóm 2-3 dòng/bài là suy diễn từ tiêu đề, raw không có mô tả`.
- `12-crawl-chi-tiet-5-links.md`: fix đúng 1 ký tự trong bullet registration — `chuyển sang NGỌNG` → `chuyến sang NGỌNG` (khớp nguyên văn raw `...trước khi chuyến sang NGỌNG...`, dù là typo gốc); không đụng chỗ khác.
