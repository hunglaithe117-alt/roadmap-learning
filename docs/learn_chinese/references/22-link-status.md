# Trạng thái links tài liệu ngoài (kiểm tra 2026-09-23)

> Kiểm bằng `curl -L --max-time 15`, UA trình duyệt Chrome 126, chạy song song
> `xargs -P8`; URL nào timeout được retry (30s → 45s → 60s) cho tới khi ổn định
> rồi mới chốt. Đối tượng: 142 URL trần trong bảng của `docs/06-tai-lieu-tham-khao.md`.
> Mã 403 = site chặn bot nhưng trang vẫn mở được bằng trình duyệt thật. Mã 000 =
> không kết nối được lúc kiểm (timeout hoặc lỗi DNS) — thử lại bằng trình duyệt.
> Dùng cho cột **Trạng thái** và disclaimer `[Tham khảo thêm]`.

## Kết quả tổng hợp

| Nhóm | Số URL | Tỷ lệ |
|---|---|---|
| Sống (2xx) | 134 | 94,4% |
| Bot-block (403) | 4 | 2,8% |
| 404 | 2 | 1,4% |
| Lỗi kết nối (000) | 2 | 1,4% |
| **Tổng** | **142** | **100%** |

- **Không có URL redirect vòng** — số lần redirect lớn nhất ghi nhận là 3 hop.
- Danh sách 134 URL sống không liệt kê ở đây; xem từng dòng trong cột
  **Trạng thái** của `docs/06-tai-lieu-tham-khao.md`.

## URL lỗi — Bot-block (403)

| URL | Ghi chú |
|---|---|
| https://doi.org/10.1075/jslp.22033.lu | redirect 1 hop sang `http://www.jbe-platform.com/content/journals/10.1075/jslp.22033.lu`, chặn bot — mở trình duyệt dùng bình thường |
| https://doi.org/10.5281/zenodo.15549790 | redirect 1 hop sang `https://zenodo.org/doi/10.5281/zenodo.15549790`, chặn bot — mở trình duyệt dùng bình thường |
| https://www.sciencedirect.com/science/article/abs/pii/S0883035521000100 | chặn bot (trang abstract cả năm trả 403 cho curl) — mở trình duyệt dùng bình thường |
| https://informasilengkap.com/en/CEDICT | chặn bot — mở trình duyệt dùng bình thường; nguồn thứ cấp, đã ghi trong gap F8 |

## URL lỗi — 404

| URL | Ghi chú |
|---|---|
| http://hanzidb.org/character-list/hsk | trả 404 — đây là nguồn trích "số chữ suy từ wordlist" (F1); cần tìm bản mirror hoặc ghi lại gap |
| https://sites.lynu.edu.cn/yywz/info/1003/1041.htm | trả 404 — bản text GB/T 16159 (洛陽師範); bản pinyin.info và 国家标准馆 vẫn sống nên quy tắc标原调不标变调 không mất nguồn |

## URL lỗi — Lỗi kết nối (000)

| URL | Ghi chú |
|---|---|
| http://ica.org.vn/anh-huong-cua-chuyen-di-tieu-cuc-tu-tieng-me-de-den-phat-am-tieng-trung-cua-nguoi-hoc-viet-nam-bang-chung-tu-khao-sat-va-can-thiep-ngu-am/ | timeout (curl rc=28) qua 4 lần thử, 15s→60s — nguồn của F3 (n=42, 40,6%); tra lại bằng trình duyệt trước khi trích |
| https://lllc.uiowa.edu/sites/lllc.uiowa.edu/files/2025-05/hsk_manual_chinese_and_english.pdf | timeout (curl rc=28) qua 4 lần thử — mirror HSK/HSKK Handbook, nguồn xác nhận HSKK 60/100 (F5); tra lại bằng trình duyệt trước khi trích |

## Hành động

- 4 URL 403: giữ nguyên, cột Trạng thái ghi `bot-block` — không dùng để claim dữ
  kiện, đã có sẵn disclaimer `[Tham khảo thêm]`.
- 2 URL 404 + 2 URL 000: **không** gỡ khỏi index; cột Trạng thái ghi rõ `404` /
  `lỗi (000)` để người đọc tự tra lại. Không thay bằng URL mới — nguồn research
  đang đóng băng ngày 2026-09-23 (xem `docs/00-gioi-han-du-lieu-va-gia-dinh.md`).
