# Trạng thái links tài liệu ngoài (kiểm tra 2026-09-23)

> Kiểm bằng curl (follow redirect, timeout 8s/URL). Mã 403 = site chặn bot nhưng
> trang vẫn tồn tại với trình duyệt thật. Mã 000 = không kết nối được lúc kiểm tra
> (thử lại bằng trình duyệt). Dùng cho disclaimer `[Tham khảo thêm]` ở Phase 2.

## Kết quả

| Mã | URL | Ghi chú |
|---|---|---|
| 200 | https://www.bbc.co.uk/learningenglish | sống |
| 200 | https://www.youtube.com/@bbclearningenglish | sống |
| 200 | https://learningenglish.voanews.com | sống |
| 200 | https://www.youtube.com/@voalearningenglish | sống |
| 200 | https://learningenglish.voanews.com/z/1689 | sống |
| 200 | https://www.rachelsenglish.com | sống |
| 200 | https://www.youtube.com/@RachelsEnglish | sống |
| 403 | https://dictionary.cambridge.org | chặn bot, mở trình duyệt dùng bình thường |
| 200 | https://youglish.com | sống |
| 403 | https://forvo.com | chặn bot, mở trình duyệt dùng bình thường |
| 404 | https://www.youtube.com/@JenniferESL | handle @ sai — tìm "JenniferESL" trực tiếp trên YouTube thay vì dùng link này |
| 200 | https://speechling.com | sống |
| 200 | https://apps.ankiweb.net | sống |
| 200 | https://teacherluke.co.uk | sống |
| 200 | https://www.oxfordlearnersdictionaries.com/ | sống |
| 200 | https://ozdic.com/ | sống |
| 200 | http://www.just-the-word.com/ | sống |
| 200 | https://www.etymonline.com/ | sống |
| 403 | https://www.english-corpora.org/coca/ | chặn bot, mở trình duyệt dùng bình thường |
| 200 | https://skell.sketchengine.eu/ | sống |
| 403 | https://ludwig.guru/ | chặn bot, mở trình duyệt dùng bình thường |
| 200 | https://owl.purdue.edu/ | sống |
| 200 | https://owl.purdue.edu/owl/general_writing/academic_writing/historical_perspectives_on_argumentation/toulmin_argument.html | sống |
| 200 | https://www.phrasebank.manchester.ac.uk/ | sống |
| 200 | https://writingcenter.unc.edu/tips-and-tools/ | sống |
| 200 | http://www.uefap.com/ | sống |
| 000 | https://learnenglish.britishcouncil.org/ | không kết nối lúc kiểm tra — thử lại bằng trình duyệt |
| 200 | https://www.thesaurus.com/ | sống |
| 200 | https://www.powerthesaurus.org/ | sống |
| 403 | https://quizlet.com/ | chặn bot, mở trình duyệt dùng bình thường |
| 200 | https://www.tesol.org/ | sống |
| 000 | https://www.teachingenglish.org.uk/ | không kết nối lúc kiểm tra — thử lại bằng trình duyệt |
| 200 | https://www.ielts.org/ | sống |
| 200 | https://www.cambridge.org/elt/blog/2019/08/12/learning-learn-flash-cards-spaced-repetition-example-sentences/ | sống |
| 200 | https://www.colby.edu/wp-content/uploads/2024/10/Kaplan_CR_1965.pdf | sống (PDF Kaplan) |
| 200 | https://www.sketchengine.eu/user-guide/students/ | sống |
| 200 | https://docs.ankiweb.net/background.html | sống |
| 200 | https://journals.plos.org/plosone/article?id=10.1371/journal.pone.0120644 | sống |
| 200 | https://educationusa.state.gov/ | sống |

## Hành động cho Phase 2

- Sửa link JenniferESL: bỏ URL @, ghi "tìm kênh JenniferESL trên YouTube".
- Links 403/000 giữ lại kèm disclaimer đúng trạng thái (bot-block / thử lại).
