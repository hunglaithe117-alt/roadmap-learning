// Bài đọc graded bundle sẵn. Port NGUYÊN VĂN từ api/reader.go — nội dung
// bài học, đổi câu là đổi bài.
//
// Toàn bộ TỰ BIÊN cho app học (câu ngắn HSK1-2 / A1-A2), không sao chép sách
// hay kịch bản nào; mỗi bài ghi rõ `Source` để UI hiện nguồn.
package contentinfra

// ReaderArticle là 1 bài đọc phân loại trình độ, bundle sẵn trong code.
type ReaderArticle struct {
	ID     string
	Level  string
	Lang   string
	Title  string
	Text   string
	Source string
}

const readerSource = "Tự biên cho app học (không bản quyền, public-domain equivalent)"

// readerArticles: 8 bài ngắn tự biên (2 bài × HSK1/HSK2/A1/A2).
var readerArticles = []ReaderArticle{
	{
		ID: "hsk1-1", Level: "HSK1", Lang: "zh",
		Title:  "我的一天",
		Text:   "我叫小明。我是学生。我每天去学校。我爱学习中文。我有一个朋友。他叫大力。我们一起喝茶。",
		Source: readerSource,
	},
	{
		ID: "hsk1-2", Level: "HSK1", Lang: "zh",
		Title:  "我的家",
		Text:   "我家有三个人。我、妈妈和爸爸。妈妈爱吃米饭。爸爸爱喝茶。我爱我的家。",
		Source: readerSource,
	},
	{
		ID: "hsk2-1", Level: "HSK2", Lang: "zh",
		Title:  "去学校",
		Text:   "早上七点，我去学校。学校很大，学生很多。老师教我们中文。学习中文很有意思。我每天都很高兴。",
		Source: readerSource,
	},
	{
		ID: "hsk2-2", Level: "HSK2", Lang: "zh",
		Title:  "买水果",
		Text:   "昨天下午，我和朋友去买水果。苹果很红，香蕉很黄。我们买了很多水果。回家以后，我们一起吃。真好吃！",
		Source: readerSource,
	},
	{
		ID: "a1-1", Level: "A1", Lang: "en",
		Title:  "My Morning",
		Text:   "I wake up at six. I drink water. I eat bread and eggs. Then I go to school. I learn English every day.",
		Source: readerSource,
	},
	{
		ID: "a1-2", Level: "A1", Lang: "en",
		Title:  "My Family",
		Text:   "I have a small family. My father works hard. My mother cooks well. We eat dinner together. I love my family.",
		Source: readerSource,
	},
	{
		ID: "a2-1", Level: "A2", Lang: "en",
		Title:  "A Rainy Day",
		Text:   "It rains in the morning. I take my umbrella and walk to work. The streets are quiet. I pay attention to every step. After the rain, the sky is clear and I feel calm.",
		Source: readerSource,
	},
	{
		ID: "a2-2", Level: "A2", Lang: "en",
		Title:  "Keep a Promise",
		Text:   "Last week I set a small goal. I promised to learn five new words each day. I keep my promise and take notes. Now I can make progress. I share my experience with friends.",
		Source: readerSource,
	},
}

// ReaderArticles lọc theo level và id (rỗng = không lọc). `id` khớp CHÍNH XÁC
// theo hợp đồng v1, không phải tìm chuỗi con.
func ReaderArticles(level, id string) []ReaderArticle {
	out := []ReaderArticle{}
	for _, a := range readerArticles {
		if level != "" && a.Level != level {
			continue
		}
		if id != "" && a.ID != id {
			continue
		}
		out = append(out, a)
	}
	return out
}

// ReaderLevels là các trình độ có bài bundle: HSK1, HSK2, A1, A2.
var ReaderLevels = []string{"HSK1", "HSK2", "A1", "A2"}
