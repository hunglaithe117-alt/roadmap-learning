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
	{Lang: "en", Term: "photograph", Reading: "/ˈfəʊtəɡrɑːf/", Gloss: "bức ảnh"},
	{Lang: "en", Term: "photography", Reading: "/fəˈtɒɡrəfi/", Gloss: "nghệ thuật chụp ảnh"},
	{Lang: "en", Term: "photographic", Reading: "/ˌfəʊtəˈɡræfɪk/", Gloss: "thuộc về chụp ảnh"},
	{Lang: "en", Term: "progress", Reading: "/ˈprəʊɡres/ (n) · /prəˈɡres/ (v)", Gloss: "tiến bộ / tiến triển (ngoại lệ danh-động)"},
	{Lang: "en", Term: "present", Reading: "/ˈpreznt/ (n/adj) · /prɪˈzent/ (v)", Gloss: "món quà / hiện tại / trình bày (ngoại lệ)"},
	{Lang: "en", Term: "record", Reading: "/ˈrekɔːd/ (n) · /rɪˈkɔːd/ (v)", Gloss: "bản ghi / ghi lại (ngoại lệ)"},
	{Lang: "en", Term: "content", Reading: "/ˈkɒntent/ (n) · /kənˈtent/ (adj: hài lòng)", Gloss: "nội dung / hài lòng (ngoại lệ)"},
	{Lang: "en", Term: "desert", Reading: "/ˈdezət/ (n) · /dɪˈzɜːt/ (v)", Gloss: "sa mạc / bỏ đi (ngoại lệ)"},
	{Lang: "en", Term: "object", Reading: "/ˈɒbdʒɪkt/ (n) · /əbˈdʒekt/ (v)", Gloss: "vật thể / phản đối (ngoại lệ)"},
	{Lang: "en", Term: "permit", Reading: "/ˈpɜːmɪt/ (n) · /pəˈmɪt/ (v)", Gloss: "giấy phép / cho phép (ngoại lệ)"},
	{Lang: "en", Term: "produce", Reading: "/ˈprɒdjuːs/ (n) · /prəˈdjuːs/ (v)", Gloss: "nông sản / sản xuất (ngoại lệ)"},
	{Lang: "en", Term: "refuse", Reading: "/ˈrefjuːs/ (n) · /rɪˈfjuːz/ (v)", Gloss: "rác thải / từ chối (ngoại lệ)"},
	{Lang: "en", Term: "subject", Reading: "/ˈsʌbdʒɪkt/ (n) · /səbˈdʒekt/ (v)", Gloss: "chủ đề / khuất phục (ngoại lệ)"},
	{Lang: "en", Term: "contrast", Reading: "/ˈkɒntrɑːst/ (n) · /kənˈtrɑːst/ (v)", Gloss: "sự tương phản / đối chiếu (ngoại lệ)"},
	{Lang: "en", Term: "increase", Reading: "/ˈɪnkriːs/ (n) · /ɪnˈkriːs/ (v)", Gloss: "sự tăng / tăng lên (ngoại lệ)"},
	{Lang: "en", Term: "import", Reading: "/ˈɪmpɔːt/ (n) · /ɪmˈpɔːt/ (v)", Gloss: "hàng nhập / nhập khẩu (ngoại lệ)"},
	{Lang: "en", Term: "export", Reading: "/ˈekspɔːt/ (n) · /ɪkˈspɔːt/ (v)", Gloss: "hàng xuất / xuất khẩu (ngoại lệ)"},
	{Lang: "en", Term: "abandon", Reading: "/əˈbændən/", Gloss: "từ bỏ"},
	{Lang: "en", Term: "benevolent", Reading: "/bəˈnevələnt/", Gloss: "nhân từ"},
	{Lang: "en", Term: "candid", Reading: "/ˈkændɪd/", Gloss: "thẳng thắn"},
	{Lang: "en", Term: "diligent", Reading: "/ˈdɪlɪdʒənt/", Gloss: "chăm chỉ"},
	{Lang: "en", Term: "eloquent", Reading: "/ˈeləkwənt/", Gloss: "hùng biện, lưu loát"},
	{Lang: "en", Term: "frugal", Reading: "/ˈfruːɡəl/", Gloss: "tiết kiệm"},
	{Lang: "en", Term: "genuine", Reading: "/ˈdʒenjuɪn/", Gloss: "chân thật"},
	{Lang: "en", Term: "humble", Reading: "/ˈhʌmbəl/", Gloss: "khiêm tốn"},
	{Lang: "en", Term: "inevitable", Reading: "/ɪnˈevɪtəbəl/", Gloss: "không thể tránh"},
}

// SeedEnDict trả seed từ điển tích hợp.
func SeedEnDict() []SeedEntry {
	return seedEnDictEntries
}

var seedPVOCards = []SeedCard{
	{Front: "make progress", Back: "đạt tiến bộ", IPA: "/meɪk ˈprəʊɡres/", Stress: "make-PRO-gress"},
	{Front: "take responsibility", Back: "chịu trách nhiệm", IPA: "/teɪk rɪˌspɒnsəˈbɪləti/", Stress: "take-res-pon-si-BI-li-ty"},
	{Front: "keep a promise", Back: "giữ lời hứa", IPA: "/kiːp ə ˈprɒmɪs/", Stress: "keep-a-PRO-mise"},
	{Front: "break a habit", Back: "bỏ một thói quen", IPA: "/breɪk ə ˈhæbɪt/", Stress: "break-a-HA-bit"},
	{Front: "set a goal", Back: "đặt mục tiêu", IPA: "/set ə ɡəʊl/", Stress: "set-a-GOAL"},
	{Front: "reach a goal", Back: "chạm tới mục tiêu", IPA: "/riːtʃ ə ɡəʊl/", Stress: "reach-a-GOAL"},
	{Front: "pay attention", Back: "chú ý", IPA: "/peɪ əˈtenʃən/", Stress: "pay-at-TEN-tion"},
	{Front: "draw attention", Back: "thu hút sự chú ý", IPA: "/drɔː əˈtenʃən/", Stress: "draw-at-TEN-tion"},
	{Front: "solve a problem", Back: "giải quyết vấn đề", IPA: "/sɒlv ə ˈprɒbləm/", Stress: "solve-a-PRO-blem"},
	{Front: "face a challenge", Back: "đối mặt thử thách", IPA: "/feɪs ə ˈtʃælɪndʒ/", Stress: "face-a-CHA-llenge"},
	{Front: "gain experience", Back: "tích lũy kinh nghiệm", IPA: "/ɡeɪn ɪkˈspɪəriəns/", Stress: "gain-ex-PE-rien-ce"},
	{Front: "share experience", Back: "chia sẻ kinh nghiệm", IPA: "/ʃeə ɪkˈspɪəriəns/", Stress: "share-ex-PE-rien-ce"},
	{Front: "build confidence", Back: "xây dựng sự tự tin", IPA: "/bɪld ˈkɒnfɪdəns/", Stress: "build-CON-fi-dence"},
	{Front: "lose confidence", Back: "mất tự tin", IPA: "/luːz ˈkɒnfɪdəns/", Stress: "lose-CON-fi-dence"},
	{Front: "give feedback", Back: "đưa phản hồi", IPA: "/ɡɪv ˈfiːdbæk/", Stress: "give-FEED-back"},
	{Front: "receive feedback", Back: "nhận phản hồi", IPA: "/rɪˈsiːv ˈfiːdbæk/", Stress: "re-ceive-FEED-back"},
	{Front: "meet a deadline", Back: "kịp deadline", IPA: "/miːt ə ˈdedlaɪn/", Stress: "meet-a-DEAD-line"},
	{Front: "miss a deadline", Back: "trễ deadline", IPA: "/mɪs ə ˈdedlaɪn/", Stress: "miss-a-DEAD-line"},
	{Front: "save time", Back: "tiết kiệm thời gian", IPA: "/seɪv taɪm/", Stress: "save-TIME"},
	{Front: "waste time", Back: "lãng phí thời gian", IPA: "/weɪst taɪm/", Stress: "waste-TIME"},
	{Front: "learn by heart", Back: "học thuộc lòng", IPA: "/lɜːn baɪ hɑːt/", Stress: "learn-by-HEART"},
	{Front: "take notes", Back: "ghi chép", IPA: "/teɪk nəʊts/", Stress: "take-NOTES"},
	{Front: "ask a question", Back: "đặt câu hỏi", IPA: "/ɑːsk ə ˈkwestʃən/", Stress: "ask-a-QUES-tion"},
	{Front: "answer a question", Back: "trả lời câu hỏi", IPA: "/ˈɑːnsə ə ˈkwestʃən/", Stress: "AN-swer-a-QUES-tion"},
}

// SeedPVOCards trả nhóm seed tương ứng.
func SeedPVOCards() []SeedCard {
	return seedPVOCards
}

var seedPVOCardsT82 = []SeedCard{
	{Front: "make a decision", Back: "ra quyết định", IPA: "/meɪk ə dɪˈsɪʒən/", Stress: "make-a-de-CI-sion"},
	{Front: "take a break", Back: "nghỉ giải lao", IPA: "/teɪk ə breɪk/", Stress: "take-a-BREAK"},
	{Front: "have a rest", Back: "nghỉ ngơi", IPA: "/hæv ə rest/", Stress: "have-a-REST"},
	{Front: "catch a cold", Back: "bị cảm", IPA: "/kætʃ ə kəʊld/", Stress: "catch-a-COLD"},
	{Front: "have a fever", Back: "bị sốt", IPA: "/hæv ə ˈfiːvə/", Stress: "have-a-FE-ver"},
	{Front: "take medicine", Back: "uống thuốc", IPA: "/teɪk ˈmedsən/", Stress: "take-ME-di-cine"},
	{Front: "see a doctor", Back: "đi khám bệnh", IPA: "/siː ə ˈdɒktə/", Stress: "see-a-DOC-tor"},
	{Front: "make friends", Back: "kết bạn", IPA: "/meɪk frendz/", Stress: "make-FRIENDS"},
	{Front: "keep in touch", Back: "giữ liên lạc", IPA: "/kiːp ɪn tʌtʃ/", Stress: "keep-in-TOUCH"},
	{Front: "lose touch", Back: "mất liên lạc", IPA: "/luːz tʌtʃ/", Stress: "lose-TOUCH"},
	{Front: "draw a conclusion", Back: "rút ra kết luận", IPA: "/drɔː ə kənˈkluːʒən/", Stress: "draw-a-con-CLU-sion"},
	{Front: "reach an agreement", Back: "đạt thỏa thuận", IPA: "/riːtʃ ən əˈɡriːmənt/", Stress: "reach-an-a-GREE-ment"},
	{Front: "break a promise", Back: "thất hứa", IPA: "/breɪk ə ˈprɒmɪs/", Stress: "break-a-PRO-mise"},
	{Front: "tell a lie", Back: "nói dối", IPA: "/tel ə laɪ/", Stress: "tell-a-LIE"},
	{Front: "tell the truth", Back: "nói thật", IPA: "/tel ðə truːθ/", Stress: "tell-the-TRUTH"},
	{Front: "keep a secret", Back: "giữ bí mật", IPA: "/kiːp ə ˈsiːkrət/", Stress: "keep-a-SE-cret"},
	{Front: "save money", Back: "tiết kiệm tiền", IPA: "/seɪv ˈmʌni/", Stress: "save-MO-ney"},
	{Front: "spend money", Back: "tiêu tiền", IPA: "/spend ˈmʌni/", Stress: "spend-MO-ney"},
	{Front: "do homework", Back: "làm bài tập", IPA: "/duː ˈhəʊmwɜːk/", Stress: "do-HOME-work"},
	{Front: "take an exam", Back: "dự thi", IPA: "/teɪk ən ɪɡˈzæm/", Stress: "take-an-ex-AM"},
	{Front: "pass an exam", Back: "thi đỗ", IPA: "/pɑːs ən ɪɡˈzæm/", Stress: "pass-an-ex-AM"},
	{Front: "miss the bus", Back: "lỡ xe buýt", IPA: "/mɪs ðə bʌs/", Stress: "miss-the-BUS"},
	{Front: "catch the bus", Back: "đón xe buýt", IPA: "/kætʃ ðə bʌs/", Stress: "catch-the-BUS"},
	{Front: "shake hands", Back: "bắt tay", IPA: "/ʃeɪk hændz/", Stress: "shake-HANDS"},
}

// SeedPVOCardsT82 trả nhóm seed tương ứng.
func SeedPVOCardsT82() []SeedCard {
	return seedPVOCardsT82
}

var seedTMRNDCards = []SeedCard{
	{Front: "I want to make progress every day.", Back: "Tôi muốn tiến bộ mỗi ngày.", IPA: "", Stress: ""},
	{Front: "She takes notes in every lesson.", Back: "Cô ấy ghi chép trong mỗi buổi học.", IPA: "", Stress: ""},
	{Front: "We set a small goal this week.", Back: "Chúng tôi đặt một mục tiêu nhỏ tuần này.", IPA: "", Stress: ""},
	{Front: "He pays attention to stress.", Back: "Anh ấy chú ý tới trọng âm.", IPA: "", Stress: ""},
	{Front: "They share experience after class.", Back: "Họ chia sẻ kinh nghiệm sau giờ học.", IPA: "", Stress: ""},
	{Front: "I build confidence by speaking daily.", Back: "Tôi xây dựng sự tự tin bằng cách nói mỗi ngày.", IPA: "", Stress: ""},
	{Front: "She never wastes time in the morning.", Back: "Cô ấy không bao giờ lãng phí thời gian buổi sáng.", IPA: "", Stress: ""},
	{Front: "We meet the deadline together.", Back: "Chúng tôi cùng nhau kịp deadline.", IPA: "", Stress: ""},
	{Front: "He gives feedback kindly.", Back: "Anh ấy đưa phản hồi một cách tử tế.", IPA: "", Stress: ""},
	{Front: "I learn five phrases by heart.", Back: "Tôi học thuộc lòng năm cụm từ.", IPA: "", Stress: ""},
	{Front: "They face challenges with courage.", Back: "Họ đối mặt thử thách với lòng dũng cảm.", IPA: "", Stress: ""},
	{Front: "We keep our promises.", Back: "Chúng tôi giữ lời hứa.", IPA: "", Stress: ""},
}

// SeedTMRNDCards trả nhóm seed tương ứng.
func SeedTMRNDCards() []SeedCard {
	return seedTMRNDCards
}

var seedTMRNDCardsT82 = []SeedCard{
	{Front: "I take a break after lunch.", Back: "Tôi nghỉ giải lao sau bữa trưa.", IPA: "", Stress: ""},
	{Front: "She wants to see a doctor today.", Back: "Cô ấy muốn đi khám bệnh hôm nay.", IPA: "", Stress: ""},
	{Front: "We keep in touch by phone.", Back: "Chúng tôi giữ liên lạc qua điện thoại.", IPA: "", Stress: ""},
	{Front: "He always tells the truth.", Back: "Anh ấy luôn nói thật.", IPA: "", Stress: ""},
	{Front: "They reached an agreement yesterday.", Back: "Họ đã đạt thỏa thuận hôm qua.", IPA: "", Stress: ""},
	{Front: "I never break my promises.", Back: "Tôi không bao giờ thất hứa.", IPA: "", Stress: ""},
	{Front: "She saves money every month.", Back: "Cô ấy tiết kiệm tiền mỗi tháng.", IPA: "", Stress: ""},
	{Front: "We take an exam next week.", Back: "Chúng tôi dự thi vào tuần tới.", IPA: "", Stress: ""},
	{Front: "He missed the bus this morning.", Back: "Anh ấy đã lỡ xe buýt sáng nay.", IPA: "", Stress: ""},
	{Front: "They shake hands before the match.", Back: "Họ bắt tay trước trận đấu.", IPA: "", Stress: ""},
	{Front: "I do my homework in the evening.", Back: "Tôi làm bài tập vào buổi tối.", IPA: "", Stress: ""},
	{Front: "She keeps every secret well.", Back: "Cô ấy giữ mọi bí mật rất tốt.", IPA: "", Stress: ""},
}

// SeedTMRNDCardsT82 trả nhóm seed tương ứng.
func SeedTMRNDCardsT82() []SeedCard {
	return seedTMRNDCardsT82
}
