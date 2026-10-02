// Khoá localStorage mà `streak.ts` dùng, tách ra để test Dashboard import được
// mà không phải chép chuỗi ở 4 nơi (chép lệch 1 ký tự là test xanh nhầm).
//
// `streak.ts` khai khoá nội bộ (`const LAST_VISIT_KEY = 'last-visit-day'`), nên
// ở đây khai lại đúng giá trị đó. Nếu `streak.ts` đổi khoá mà quên sửa file
// này thì test `test_hien_banner_roi_moi_danh_dau_da_ghe` đỏ — đó là chủ ý.
export const LAST_VISIT_KEY_FOR_TEST = 'last-visit-day';
