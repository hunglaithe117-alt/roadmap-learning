# 00 — Giới hạn dữ liệu và giả định

File này đặt ranh giới của toàn bộ bộ tài liệu. Đọc trước khi dùng [01](./01-lo-trinh-tong-quan.md).

## Nguồn đóng băng

- Đóng băng ngày **2026-09-23**, đúng 3 file research trong `references/`:
  1. `10-chuan-hsk-chuong-trinh.md`
  2. `11-pinyin-thanh-dieu-nghe-noi.md`
  3. `12-chu-han-tu-vung-ngu-phap.md`
- Nhãn **[Nguồn: …]** trong mọi file docs chỉ được trích lại từ 3 file này (giữ nguyên URL + trích nguyên văn ngắn). Thêm URL mới vào nhãn Nguồn = vi phạm đóng băng.

## Ba nhãn (mutually exclusive — mỗi claim đúng một nhãn)

1. **[Nguồn: \<tên\> + \<URL\> + trích "\<nguyên văn ngắn\>"]** — dữ kiện có trong 3 file đóng băng.
   - Ví dụ: [Nguồn: HSK Guide + https://www.hsk.guide/curriculum + trích "HSK 1 … words 150 characters 174"].
   - Số liệu rút ra bằng phép trừ/gộp từ bảng nguồn cũng vẫn là Nguồn, nhưng phải ghi rõ là suy từ bảng (xem F1: "số chữ suy từ wordlist").
2. **[Suy luận self-learn + giả định: …]** — cách tự học, tiêu chí qua giai đoạn, mọi ước lượng thời lượng. Không phải lời nguồn.
   - Ví dụ: [Suy luận self-learn + giả định: 30–45 phút/ngày].
3. **[Tham khảo thêm: \<tên\> + \<URL\> + \<loại\>]** — link ngoài để tự tra, không phải endorsement, không dùng để claim dữ kiện.
   - Ví dụ: [Tham khảo thêm: Nottingham Malaysia — HSK & HSKK exam structure + https://www.nottingham.edu.my/Education/Programmes/hsk-hskk-chinese-test.aspx + trang tổng hợp cộng đồng].

Một claim không được gắn đồng thời hai nhãn. Mất chắc chắn thì ghi **gap** (mục dưới), không suy đoán.

## Giả định vận hành

- [Suy luận self-learn + giả định: nhịp 30–45 phút/ngày] Mọi thời lượng trong [01](./01-lo-trinh-tong-quan.md) là ước lượng ở nhịp này, để duy trì đều đặn — không phải yêu cầu của nguồn nào và không phải lịch trình cố định.
- [Suy luận self-learn + giả định: không giáo viên, không lớp] Tiêu chí "qua giai đoạn" đều là tự kiểm (tự ghi âm, tự làm mock, tự tra từ điển), không phụ thuộc người chấm.
- Bộ tài liệu **không** chứa: lịch trình theo ngày/buổi, học phí, lệ phí, logistics đăng ký/lớp học — nếu cần thì ra ngoài phạm vi và phải tự kiểm lại nguồn hiện hành.

## Quy tắc số liệu F1–F8 (Gate 1 — áp dụng mọi số liệu trong 3 file docs)

- **F1 — HSK 2.0:** bảng duy nhất = 150 / 300 / 600 / 1.200 từ; 174 / 347 / 617 / 1.064 chữ, ghi rõ **"số chữ suy từ wordlist"** (không có bảng chữ chính thức riêng). Variant 348 / 618 (chill-chinese) chỉ nằm ở footnote, không thành chuẩn. Bảng đầy đủ: [README](./README.md).
- **F2 — HSK 3.0:** chỉ dùng bộ chính thức 2025 (file 10): tích lũy 300 / 500 / 1.000 / 2.000 / 3.600 / 5.400 / 11.000; số bản nháp 2021 (500 / 1.272 / 2.245 / 3.245) vào footnote. Bắt buộc phân biệt **"tích lũy"** vs **"chữ mới mỗi cấp"** (chữ mới HSK 3.0: 246 / 125 / 284 / 441 theo HanziFeed).
- **F3 — lỗi thanh người Việt:** cấm nói chung chung một tỉ lệ lỗi thanh bao trùm cho "người Việt" (dạng quy tròn, tách khỏi nghiên cứu). Chỉ dùng đúng dạng: **"1 nghiên cứu n=42 ghi 40,6%"** (40,6% trong tổng 384 lỗi, [Nguồn: ICA + http://ica.org.vn/anh-huong-cua-chuyen-di-tieu-cuc-tu-tieng-me-de-den-phat-am-tieng-trung-cua-nguoi-hoc-viet-nam-bang-chung-tu-khao-sat-va-can-thiep-ngu-am/ + trích "Thanh điệu: 156 lỗi (40,6%)"]).
- **F4 — HSKK Intermediate ≈900 từ:** ghi là **"gợi ý, chưa kiểm"** — file 10 dẫn "cùng nguồn iysc", không kèm trích nguyên văn cho con số này: [Nguồn: iysc + http://www.iysc.org/the-overall-hskk-guide.html].
- **F5 — điểm đạt / hiệu lực 2 năm:** nguồn dẫn là IndiaChinaAcademy (community) → **hạ tin cậy**, ưu tiên HSK/HSKK handbook (mirror trong file 11/10 — handbook xác nhận được HSKK 60/100: [Nguồn: HSK/HSKK Handbook mirror + https://lllc.uiowa.edu/sites/lllc.uiowa.edu/files/2025-05/hsk_manual_chinese_and_english.pdf + trích "HSKK（初级）满分100分，总分60分为合格。"]). Điểm 120/200 (HSK 1–2) và 180/300 (HSK 3–4) chưa đối chiếu handbook trong 3 file → ghi "chưa đối chiếu handbook" khi nhắc.
- **F6 — timeline HSK 3.0** (thử nghiệm 31/01/2026 và 20/09/2026, ra mắt 13/12/2026…): ghi **"theo CTI 2026, kiểm lại trước khi đăng ký"**; **không** dùng làm giả định cứng cho lộ trình G0–G4.
- **F7 — ngữ pháp 54 / 79 / 87 / 115 điểm (HSK 1–4):** ghi **"ước tính AllSet, chưa từng công bố chính thức"** — chính AllSet cũng nói đây là estimate: [Nguồn: AllSet + http://resources.allsetlearning.com/chinese/grammar/2012_HSK_grammar_points + trích "NOTE: These HSK levels are estimates, since the 2012 HSK standard never officially released a clear standard for grammar points."].
- **F8 — quy tắc luyện tập:** 1-3-7-14-30 ngày, "10–30 encounters" chỉ ghi là **"quy tắc thực hành phổ biến"** / **"claim của hãng"** (claim của trang Mandarin Companion), không thành chuẩn khoa học. **Bỏ con số quy mô entry của CC-CEDICT** (nguồn thứ cấp, không trích được từ trang chính thức) — con số đó không xuất hiện ở bất kỳ file docs nào.
- **Tổng hợp biên tập:** các mục tổng hợp trong file 10 (dòng 95–127: "Kết luận chọn sách", "Thứ tự chốt cho người mới số 0") là **tổng hợp biên tập của file nguồn, không phải trích nguyên văn** — khi dùng phải ghi rõ, không dán nhãn [Nguồn + trích] cho phần kết luận đó.

## Gap đã biết (thành thật — thiếu thì ghi, không bịa)

1. **Không có chuẩn audio bản ngữ kiểm chứng.** Các claim "native audio" (hskcore, Pleco 34.000+ recordings…) là claim của hãng/deck, không có chuẩn độc lập trong 3 file.
2. **Không có thông tin điểm thi / lệ phí tại Việt Nam.** File 10 chỉ có ví dụ phí ở vài điểm thi Trung Quốc (NUIST: HSK1 150 CNY … HSK4 450 CNY) và ghi "không trích được biểu phí chung chính thức"; lệ phí/điểm thi tại VN = gap.
3. **Bộ thủ 214 = bảng kiểu Khang Hy, cho chữ phồn thể** — trích nguồn là "all 214 radicals for traditional Chinese characters" ([Nguồn: Arch Chinese + https://archchinese.com/traditional_chinese_radicals.html + trích "The Chinese Radical Table lists all 214 radicals for traditional Chinese characters…"]). Khi nhắc phải ghi rõ đây là bảng phồn thể, không phải bảng riêng cho giản thể.
4. Format đề HSK 3.0 chi tiết từng cấp: file 10 ghi **không trích được** từ trang chính thức.
5. Hình thức "thi HSK online tại nhà" (home edition): **không trích được** nguồn xác nhận còn hiệu lực 2026.
6. URL trực tiếp danh sách từ HSK chính thức (CLEC/Hanban): file 12 ghi **không trích được**; các bảng HSK trong nguồn lấy từ trang tổng hợp.
7. Rubric HSKK: Hanban công bố rubric theo nhiệm vụ nhưng **không công bố phân bổ điểm**, chưa có nghiên cứu validate ([Nguồn: Sage review + https://sage.cnpereading.com/doi/10.1177/02655322231163470 + trích "no information has been provided and no study appears to have validated the design of the rubric"]). % trọng số của Viện Khổng Tử Groningen (60/30/10) không tìm thấy trong văn bản chính thức.
8. Nghiên cứu lỗi **ü** và **r** của người Việt: file 11 ghi **không tìm được** (chỉ có dữ kiện gián tiếp).
9. Số bài của HSK Standard Course 2–4: file nguồn chỉ trích được "15 lessons" cho cấp 1 → các cấp khác = gap khi ước lượng.
10. Không trích được: mô tả chính thức dạng text của Mandarin Corner; catalog chính thức NXB của Chinese Breeze; mô tả chi tiết một số deck Anki (Heisig, deck vẽ nét HSK 3.0); bảng tần suất chính thức.
11. **Bỏ:** mọi con số quy mô entry của CC-CEDICT (F8) — nguồn thứ cấp, không kiểm được từ trang chính thức.

Về [README](./README.md) | Tiếp: [01](./01-lo-trinh-tong-quan.md).
