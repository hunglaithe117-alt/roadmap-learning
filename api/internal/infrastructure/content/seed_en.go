// Seed dữ liệu tiếng Anh cho context content. Port NGUYÊN VĂN từ
// api/english.go — dữ liệu này là nội dung học thuật, đổi 1 cụm là đổi bài
// tập.
//
// Toàn bộ nghĩa tiếng Việt và IPA do tác giả app soạn cho mục đích drill,
// KHÔNG sao chép từ từ điển nào (giữ nguyên tuyên bố của v1).
package contentinfra

// SeedEntry là 1 dòng seed en_dict (headword + IPA + nghĩa).
type SeedEntry struct {
	Lang    string
	Term    string
	Reading string
	Gloss   string
}

// SeedCard là 1 thẻ seed của deck tiếng Anh: cụm PVO (verb+object) hoặc câu
// ngắn TMRND để luyện shadowing/chunking. IPA/Stress rỗng với câu TMRND.
type SeedCard struct {
	Front  string
	Back   string
	IPA    string
	Stress string
}

// seedEnDictEntries — từ điển tiếng Anh tích hợp.
var seedEnDictEntries = []SeedEntry{
	{"en", "photograph", "/ˈfəʊtəɡrɑːf/", "bức ảnh"},
	{"en", "photography", "/fəˈtɒɡrəfi/", "nghệ thuật chụp ảnh"},
	{"en", "photographic", "/ˌfəʊtəˈɡræfɪk/", "thuộc về chụp ảnh"},
	{"en", "progress", "/ˈprəʊɡres/ (n) · /prəˈɡres/ (v)", "tiến bộ / tiến triển (ngoại lệ danh-động)"},
	{"en", "present", "/ˈpreznt/ (n/adj) · /prɪˈzent/ (v)", "món quà / hiện tại / trình bày (ngoại lệ)"},
	{"en", "record", "/ˈrekɔːd/ (n) · /rɪˈkɔːd/ (v)", "bản ghi / ghi lại (ngoại lệ)"},
	{"en", "content", "/ˈkɒntent/ (n) · /kənˈtent/ (adj: hài lòng)", "nội dung / hài lòng (ngoại lệ)"},
	{"en", "desert", "/ˈdezət/ (n) · /dɪˈzɜːt/ (v)", "sa mạc / bỏ đi (ngoại lệ)"},
	{"en", "object", "/ˈɒbdʒɪkt/ (n) · /əbˈdʒekt/ (v)", "vật thể / phản đối (ngoại lệ)"},
	{"en", "permit", "/ˈpɜːmɪt/ (n) · /pəˈmɪt/ (v)", "giấy phép / cho phép (ngoại lệ)"},
	{"en", "produce", "/ˈprɒdjuːs/ (n) · /prəˈdjuːs/ (v)", "nông sản / sản xuất (ngoại lệ)"},
	{"en", "refuse", "/ˈrefjuːs/ (n) · /rɪˈfjuːz/ (v)", "rác thải / từ chối (ngoại lệ)"},
	{"en", "subject", "/ˈsʌbdʒɪkt/ (n) · /səbˈdʒekt/ (v)", "chủ đề / khuất phục (ngoại lệ)"},
	{"en", "contrast", "/ˈkɒntrɑːst/ (n) · /kənˈtrɑːst/ (v)", "sự tương phản / đối chiếu (ngoại lệ)"},
	{"en", "increase", "/ˈɪnkriːs/ (n) · /ɪnˈkriːs/ (v)", "sự tăng / tăng lên (ngoại lệ)"},
	{"en", "import", "/ˈɪmpɔːt/ (n) · /ɪmˈpɔːt/ (v)", "hàng nhập / nhập khẩu (ngoại lệ)"},
	{"en", "export", "/ˈekspɔːt/ (n) · /ɪkˈspɔːt/ (v)", "hàng xuất / xuất khẩu (ngoại lệ)"},
	{"en", "abandon", "/əˈbændən/", "từ bỏ"},
	{"en", "benevolent", "/bəˈnevələnt/", "nhân từ"},
	{"en", "candid", "/ˈkændɪd/", "thẳng thắn"},
	{"en", "diligent", "/ˈdɪlɪdʒənt/", "chăm chỉ"},
	{"en", "eloquent", "/ˈeləkwənt/", "hùng biện, lưu loát"},
	{"en", "frugal", "/ˈfruːɡəl/", "tiết kiệm"},
	{"en", "genuine", "/ˈdʒenjuɪn/", "chân thật"},
	{"en", "humble", "/ˈhʌmbəl/", "khiêm tốn"},
	{"en", "inevitable", "/ɪnˈevɪtəbəl/", "không thể tránh"},
}

// SeedEnDict trả seed từ điển tích hợp.
func SeedEnDict() []SeedEntry {
	return seedEnDictEntries
}

var seedPVOCards = []SeedCard{
	{"make progress", "đạt tiến bộ", "/meɪk ˈprəʊɡres/", "make-PRO-gress"},
	{"take responsibility", "chịu trách nhiệm", "/teɪk rɪˌspɒnsəˈbɪləti/", "take-res-pon-si-BI-li-ty"},
	{"keep a promise", "giữ lời hứa", "/kiːp ə ˈprɒmɪs/", "keep-a-PRO-mise"},
	{"break a habit", "bỏ một thói quen", "/breɪk ə ˈhæbɪt/", "break-a-HA-bit"},
	{"set a goal", "đặt mục tiêu", "/set ə ɡəʊl/", "set-a-GOAL"},
	{"reach a goal", "chạm tới mục tiêu", "/riːtʃ ə ɡəʊl/", "reach-a-GOAL"},
	{"pay attention", "chú ý", "/peɪ əˈtenʃən/", "pay-at-TEN-tion"},
	{"draw attention", "thu hút sự chú ý", "/drɔː əˈtenʃən/", "draw-at-TEN-tion"},
	{"solve a problem", "giải quyết vấn đề", "/sɒlv ə ˈprɒbləm/", "solve-a-PRO-blem"},
	{"face a challenge", "đối mặt thử thách", "/feɪs ə ˈtʃælɪndʒ/", "face-a-CHA-llenge"},
	{"gain experience", "tích lũy kinh nghiệm", "/ɡeɪn ɪkˈspɪəriəns/", "gain-ex-PE-rien-ce"},
	{"share experience", "chia sẻ kinh nghiệm", "/ʃeə ɪkˈspɪəriəns/", "share-ex-PE-rien-ce"},
	{"build confidence", "xây dựng sự tự tin", "/bɪld ˈkɒnfɪdəns/", "build-CON-fi-dence"},
	{"lose confidence", "mất tự tin", "/luːz ˈkɒnfɪdəns/", "lose-CON-fi-dence"},
	{"give feedback", "đưa phản hồi", "/ɡɪv ˈfiːdbæk/", "give-FEED-back"},
	{"receive feedback", "nhận phản hồi", "/rɪˈsiːv ˈfiːdbæk/", "re-ceive-FEED-back"},
	{"meet a deadline", "kịp deadline", "/miːt ə ˈdedlaɪn/", "meet-a-DEAD-line"},
	{"miss a deadline", "trễ deadline", "/mɪs ə ˈdedlaɪn/", "miss-a-DEAD-line"},
	{"save time", "tiết kiệm thời gian", "/seɪv taɪm/", "save-TIME"},
	{"waste time", "lãng phí thời gian", "/weɪst taɪm/", "waste-TIME"},
	{"learn by heart", "học thuộc lòng", "/lɜːn baɪ hɑːt/", "learn-by-HEART"},
	{"take notes", "ghi chép", "/teɪk nəʊts/", "take-NOTES"},
	{"ask a question", "đặt câu hỏi", "/ɑːsk ə ˈkwestʃən/", "ask-a-QUES-tion"},
	{"answer a question", "trả lời câu hỏi", "/ˈɑːnsə ə ˈkwestʃən/", "AN-swer-a-QUES-tion"},
}

// SeedPVOCards trả nhóm seed tương ứng.
func SeedPVOCards() []SeedCard {
	return seedPVOCards
}

var seedPVOCardsT82 = []SeedCard{
	{"make a decision", "ra quyết định", "/meɪk ə dɪˈsɪʒən/", "make-a-de-CI-sion"},
	{"take a break", "nghỉ giải lao", "/teɪk ə breɪk/", "take-a-BREAK"},
	{"have a rest", "nghỉ ngơi", "/hæv ə rest/", "have-a-REST"},
	{"catch a cold", "bị cảm", "/kætʃ ə kəʊld/", "catch-a-COLD"},
	{"have a fever", "bị sốt", "/hæv ə ˈfiːvə/", "have-a-FE-ver"},
	{"take medicine", "uống thuốc", "/teɪk ˈmedsən/", "take-ME-di-cine"},
	{"see a doctor", "đi khám bệnh", "/siː ə ˈdɒktə/", "see-a-DOC-tor"},
	{"make friends", "kết bạn", "/meɪk frendz/", "make-FRIENDS"},
	{"keep in touch", "giữ liên lạc", "/kiːp ɪn tʌtʃ/", "keep-in-TOUCH"},
	{"lose touch", "mất liên lạc", "/luːz tʌtʃ/", "lose-TOUCH"},
	{"draw a conclusion", "rút ra kết luận", "/drɔː ə kənˈkluːʒən/", "draw-a-con-CLU-sion"},
	{"reach an agreement", "đạt thỏa thuận", "/riːtʃ ən əˈɡriːmənt/", "reach-an-a-GREE-ment"},
	{"break a promise", "thất hứa", "/breɪk ə ˈprɒmɪs/", "break-a-PRO-mise"},
	{"tell a lie", "nói dối", "/tel ə laɪ/", "tell-a-LIE"},
	{"tell the truth", "nói thật", "/tel ðə truːθ/", "tell-the-TRUTH"},
	{"keep a secret", "giữ bí mật", "/kiːp ə ˈsiːkrət/", "keep-a-SE-cret"},
	{"save money", "tiết kiệm tiền", "/seɪv ˈmʌni/", "save-MO-ney"},
	{"spend money", "tiêu tiền", "/spend ˈmʌni/", "spend-MO-ney"},
	{"do homework", "làm bài tập", "/duː ˈhəʊmwɜːk/", "do-HOME-work"},
	{"take an exam", "dự thi", "/teɪk ən ɪɡˈzæm/", "take-an-ex-AM"},
	{"pass an exam", "thi đỗ", "/pɑːs ən ɪɡˈzæm/", "pass-an-ex-AM"},
	{"miss the bus", "lỡ xe buýt", "/mɪs ðə bʌs/", "miss-the-BUS"},
	{"catch the bus", "đón xe buýt", "/kætʃ ðə bʌs/", "catch-the-BUS"},
	{"shake hands", "bắt tay", "/ʃeɪk hændz/", "shake-HANDS"},
}

// SeedPVOCardsT82 trả nhóm seed tương ứng.
func SeedPVOCardsT82() []SeedCard {
	return seedPVOCardsT82
}

var seedTMRNDCards = []SeedCard{
	{"I want to make progress every day.", "Tôi muốn tiến bộ mỗi ngày.", "", ""},
	{"She takes notes in every lesson.", "Cô ấy ghi chép trong mỗi buổi học.", "", ""},
	{"We set a small goal this week.", "Chúng tôi đặt một mục tiêu nhỏ tuần này.", "", ""},
	{"He pays attention to stress.", "Anh ấy chú ý tới trọng âm.", "", ""},
	{"They share experience after class.", "Họ chia sẻ kinh nghiệm sau giờ học.", "", ""},
	{"I build confidence by speaking daily.", "Tôi xây dựng sự tự tin bằng cách nói mỗi ngày.", "", ""},
	{"She never wastes time in the morning.", "Cô ấy không bao giờ lãng phí thời gian buổi sáng.", "", ""},
	{"We meet the deadline together.", "Chúng tôi cùng nhau kịp deadline.", "", ""},
	{"He gives feedback kindly.", "Anh ấy đưa phản hồi một cách tử tế.", "", ""},
	{"I learn five phrases by heart.", "Tôi học thuộc lòng năm cụm từ.", "", ""},
	{"They face challenges with courage.", "Họ đối mặt thử thách với lòng dũng cảm.", "", ""},
	{"We keep our promises.", "Chúng tôi giữ lời hứa.", "", ""},
}

// SeedTMRNDCards trả nhóm seed tương ứng.
func SeedTMRNDCards() []SeedCard {
	return seedTMRNDCards
}

var seedTMRNDCardsT82 = []SeedCard{
	{"I take a break after lunch.", "Tôi nghỉ giải lao sau bữa trưa.", "", ""},
	{"She wants to see a doctor today.", "Cô ấy muốn đi khám bệnh hôm nay.", "", ""},
	{"We keep in touch by phone.", "Chúng tôi giữ liên lạc qua điện thoại.", "", ""},
	{"He always tells the truth.", "Anh ấy luôn nói thật.", "", ""},
	{"They reached an agreement yesterday.", "Họ đã đạt thỏa thuận hôm qua.", "", ""},
	{"I never break my promises.", "Tôi không bao giờ thất hứa.", "", ""},
	{"She saves money every month.", "Cô ấy tiết kiệm tiền mỗi tháng.", "", ""},
	{"We take an exam next week.", "Chúng tôi dự thi vào tuần tới.", "", ""},
	{"He missed the bus this morning.", "Anh ấy đã lỡ xe buýt sáng nay.", "", ""},
	{"They shake hands before the match.", "Họ bắt tay trước trận đấu.", "", ""},
	{"I do my homework in the evening.", "Tôi làm bài tập vào buổi tối.", "", ""},
	{"She keeps every secret well.", "Cô ấy giữ mọi bí mật rất tốt.", "", ""},
}

// SeedTMRNDCardsT82 trả nhóm seed tương ứng.
func SeedTMRNDCardsT82() []SeedCard {
	return seedTMRNDCardsT82
}
