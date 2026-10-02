package main

import domaincontent "langapp/internal/domain/content"

// Audio-service DÙNG CHUNG bảng phồn→giản với app chính.
//
// M4 ban đầu để bản sao riêng ở đây với lập luận "tiến trình audio không được
// import tầng trong của app". Lập luận đó SAI về kỹ thuật: audio-service cùng
// module `langapp`, và quy tắc `internal/` của Go chỉ chặn import từ NGOÀI module
// — nó không có tác dụng gì với `services/`. Hệ quả là 2 bảng 100 mục giống
// hệt không có test nào ràng buộc, nên lệch bảng là chấm sai oan: `practice.
// DiffAgainstSample` dùng `domain.ToSimplified` còn audio-service dùng bản của
// nó, và Whisper trả "學習" thì app chấm đúng còn service đã đổi — hoặc ngược lại.
//
// `domain/content.ToSimplified` là hàm thuần Go (chỉ có bảng rune tự chứa, KHÔNG
// dùng `unicode/norm` vì GOROOT máy build bị cắt mất package đó — STACK-V2 §8),
// nên import không kéo theo driver hay framework nào.
func toSimplified(s string) string { return domaincontent.ToSimplified(s) }

// hasTraditional báo chuỗi còn chữ phồn. `domain/content.HasTraditional` làm
// đúng việc này, nên dùng luôn thay vì viết bản thứ hai.
func hasTraditional(s string) bool { return domaincontent.HasTraditional(s) }
