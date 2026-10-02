package practice

import (
	"errors"
	"time"
)

// Giới hạn tốc độ phát trong player: 0.5x-1.5x.
const (
	MinRate = 0.5
	MaxRate = 1.5
	// DefaultRate là tốc độ khi người dùng không gửi rate.
	DefaultRate = 1.0
)

// ErrRateOutOfRange là lỗi thuần khi rate nằm ngoài khoảng cho phép —
// transport map sang 400 bằng errors.As.
var ErrRateOutOfRange = errors.New("rate chỉ từ 0.5 đến 1.5")

// NormalizeRate chuẩn hóa rate: 0 (client không gửi) -> DefaultRate; ngoài
// [MinRate, MaxRate] -> lỗi. rate âm cũng là lỗi, không phải mặc định.
func NormalizeRate(rate float64) (float64, error) {
	if rate == 0 {
		return DefaultRate, nil
	}
	if rate < MinRate || rate > MaxRate {
		return 0, ErrRateOutOfRange
	}
	return rate, nil
}

// ErrNegativeLoops là lỗi khi số vòng lặp âm — vòng lặp là bộ đếm, không
// thể âm.
var ErrNegativeLoops = errors.New("loops không được âm")

// ValidateLoops kiểm tra số vòng lặp.
func ValidateLoops(loops int) error {
	if loops < 0 {
		return ErrNegativeLoops
	}
	return nil
}

// AdvanceShadowSession là bước A-B loop: sau mỗi vòng nghe lại, số vòng tăng
// 1 và rate được chuẩn hóa. Trả session mới — không mutate input.
func AdvanceShadowSession(cur ShadowSession, rate float64, now time.Time) (ShadowSession, error) {
	if err := ValidateLoops(cur.Loops + 1); err != nil {
		return ShadowSession{}, err
	}
	normalized, err := NormalizeRate(rate)
	if err != nil {
		return ShadowSession{}, err
	}
	return ShadowSession{
		CardID:    cur.CardID,
		Loops:     cur.Loops + 1,
		Rate:      normalized,
		UpdatedAt: now.UTC(),
	}, nil
}

// NewShadowSession là session đầu tiên cho 1 thẻ: 0 vòng, rate mặc định,
// UpdatedAt zero (chưa có lần ghi nào).
func NewShadowSession(cardID int64) ShadowSession {
	return ShadowSession{CardID: cardID, Loops: 0, Rate: DefaultRate}
}
