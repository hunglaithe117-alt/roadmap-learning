# Phase 8 — Content (HSK seed + PVO expand)

## Goal

Nạp 1200 từ HSK 2.0 L1-L4 có nghĩa Việt qua endpoint import có sẵn (idempotent),
rồi mở rộng bộ PVO/TMRND tự biên theo mẫu T3.2 (đã có 24+12).

## Dependencies

- v1 đã xong (endpoint import + SQLite FTS5). Phase-8 sau v1, độc lập phase-7/9.

## Exit criteria

- 1200 từ HSK L1-L4 trong DB, import lại idempotent không trùng, mỗi từ có nghĩa
  Việt tự soạn + pinyin đối chiếu.
- Bộ PVO/TMRND mở rộng thêm theo mẫu T3.2, hiển thị được trong SPA.

## Task list

- `tasks/T8.1-hsk-seed.md` — 6h
- `tasks/T8.2-pvo-expand.md` — 6h

## Parallelization

- T8.1 trước T8.2 nếu cùng người (chung pipeline import/seed); khác người thì song
  song được.
- Phase-7/8/9 song song được sau khi duyệt plan (độc lập nhau, chỉ chung API/health).

## Design-doc references

- `phases/SOLUTION-v2.md` (mục 8: drkameleon chính, clem109 backup, hsk30 để sau)
- `phases/phase-3-english/tasks/T3.2-*` (mẫu PVO/TMRND tự biên 24+12)

## Risk notes

- License: nguồn HSK MIT (drkameleon/clem109 bundle OK, giữ copyright +
  attribution); nghĩa Việt tự soạn, pinyin đối chiếu — KHÔNG copy tài liệu bản
  quyền (giáo trình, ví dụ có bản quyền).
- HSK 3.0 đầy đủ (ivankra/hsk30) để sau, không nhồi vào phase này gây phình scope.
