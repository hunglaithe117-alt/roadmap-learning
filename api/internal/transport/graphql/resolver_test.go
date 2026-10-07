package graphql_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/stretchr/testify/require"

	contentapp "langapp/internal/application/content"
	roadmapapp "langapp/internal/application/roadmap"
	srsapp "langapp/internal/application/srs"
)

type userError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// ── srs ─────────────────────────────────────────────────────────────────────

func Test_create_deck_and_card_returns_guid(t *testing.T) {
	h := newHarness(t)
	var resp struct {
		CreateDeck struct {
			Ok   bool `json:"ok"`
			Deck struct {
				ID, GUID, Name, Lang string
			} `json:"deck"`
			Error *userError `json:"error"`
		} `json:"createDeck"`
	}
	h.client(t).MustPost(
		`mutation($n: String!, $l: String) { createDeck(name: $n, lang: $l) { ok deck { id guid name lang } error { message code } } }`,
		&resp, client.Var("n", "HSK1"), client.Var("l", "zh"))

	require.True(t, resp.CreateDeck.Ok)
	require.Nil(t, resp.CreateDeck.Error)
	require.NotEmpty(t, resp.CreateDeck.Deck.GUID, "mọi type phải có guid")
	require.Equal(t, "HSK1", resp.CreateDeck.Deck.Name)
	require.Equal(t, "zh", resp.CreateDeck.Deck.Lang)
}

func Test_create_deck_rejects_unknown_lang_with_400_message(t *testing.T) {
	h := newHarness(t)
	var resp struct {
		CreateDeck struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"createDeck"`
	}
	h.client(t).MustPost(
		`mutation { createDeck(name: "X", lang: "klingon") { ok error { message code } } }`, &resp)

	require.False(t, resp.CreateDeck.Ok)
	require.NotNil(t, resp.CreateDeck.Error)
	require.Equal(t, "BAD_REQUEST", resp.CreateDeck.Error.Code)
	require.NotEmpty(t, resp.CreateDeck.Error.Message)
}

// Lỗi nghiệp vụ phải mang message TIẾNG VIỆT NGUYÊN VĂN của tầng application —
// transport không dịch lại (quy ước M4). Test này chốt đúng điều đó.
func Test_business_error_message_is_verbatim_from_application(t *testing.T) {
	h := newHarness(t)
	var resp struct {
		DeleteStage struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"deleteStage"`
	}
	// Lấy message từ CHÍNH use case mà mutation gọi, rồi so với message
	// GraphQL trả về. Nếu transport dịch lại (hoặc dùng message chung), 2 bên
	// lệch nhau và test đỏ.
	appMsg := h.container.Roadmap.DeleteStage(context.Background(), 999999).Error()

	h.client(t).MustPost(`mutation { deleteStage(id: "999999") { ok error { message code } } }`, &resp)

	require.False(t, resp.DeleteStage.Ok)
	require.NotNil(t, resp.DeleteStage.Error)
	require.Equal(t, appMsg, resp.DeleteStage.Error.Message,
		"message phải NGUYÊN VĂN của application, không phải bản dịch ở transport")
	require.Equal(t, "NOT_FOUND", resp.DeleteStage.Error.Code)
}

func Test_record_review_returns_schedule_and_advances_due_date(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	deck, err := h.container.SRS.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err)
	card, err := h.container.SRS.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "你", Back: "bạn", Pinyin: "ni3"})
	require.NoError(t, err)

	var resp struct {
		RecordReview struct {
			Ok     bool `json:"ok"`
			Review struct {
				DueAt        string `json:"dueAt"`
				IntervalDays int    `json:"intervalDays"`
				Fallback     bool   `json:"fallback"`
				Reps         int    `json:"reps"`
			} `json:"review"`
			Error *userError `json:"error"`
		} `json:"recordReview"`
	}
	h.client(t).MustPost(
		`mutation($id: ID!, $g: Int!) { recordReview(input: {cardId: $id, grade: $g}) { ok review { dueAt intervalDays fallback reps } error { message code } } }`,
		&resp, client.Var("id", fmt.Sprint(card.ID)), client.Var("g", 4))

	require.True(t, resp.RecordReview.Ok, "lỗi: %+v", resp.RecordReview.Error)
	require.True(t, resp.RecordReview.Review.Fallback, "reps=0 nên lịch phải rơi về bảng fallback 1-3-7-14-30")
	require.Equal(t, 1, resp.RecordReview.Review.IntervalDays, "fallback đầu tiên là 1 ngày")
	require.Equal(t, 1, resp.RecordReview.Review.Reps)
	// So theo MỐC THỜI GIAN, không so chuỗi: `newCard.dueAt` và
	// `review.dueAt` đều là "now + 1 ngày" nhưng lệch vài ms vì 2 lần gọi
	// `nowFn` khác nhau — so chuỗi sẽ đỏ oan.
	before, err := time.Parse(time.RFC3339, card.DueAt)
	require.NoError(t, err)
	after, err := time.Parse(time.RFC3339, resp.RecordReview.Review.DueAt)
	require.NoError(t, err)
	require.True(t, after.After(before) || after.Equal(before),
		"due sau ôn phải không sớm hơn due lúc tạo (%s vs %s)", after, before)
}

func Test_due_cards_includes_new_cards_even_when_due_in_future(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	deck, err := h.container.SRS.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err)
	// Thẻ mới có `due_at = now + 24h` nhưng PHẢI vào hàng đợi ôn ngay
	// (`DueFilter` luôn kéo `state = 'new'`) — hành vi v1.
	_, err = h.container.SRS.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "好", Back: "tốt"})
	require.NoError(t, err)

	var resp struct {
		DueCards []struct{ Front string } `json:"dueCards"`
	}
	h.client(t).MustPost(`query($d: ID!) { dueCards(deckId: $d) { front } }`, &resp,
		client.Var("d", fmt.Sprint(deck.ID)))

	require.Len(t, resp.DueCards, 1, "thẻ chưa từng ôn phải vào hàng đợi dù due_at ở tương lai")
	require.Equal(t, "好", resp.DueCards[0].Front)
}

// ── content ─────────────────────────────────────────────────────────────────

func Test_dict_search_returns_hanzi_entries(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, err := h.container.Content.ImportHSK(ctx, contentapp.ImportInput{Level: "HSK1"})
	require.NoError(t, err)

	var resp struct {
		DictSearch []struct {
			Hanzi, Pinyin, Nghia string
		} `json:"dictSearch"`
	}
	h.client(t).MustPost(`query { dictSearch(q: "你") { hanzi pinyin nghia } }`, &resp)

	require.NotEmpty(t, resp.DictSearch, "tra 1 ký tự Hán phải ra kết quả (đường ILIKE)")
}

func Test_import_hsk_is_idempotent_second_run_adds_nothing(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	first, err := h.container.Content.ImportHSK(ctx, contentapp.ImportInput{Level: "HSK1"})
	require.NoError(t, err)
	require.Positive(t, first.CardsAdded)

	second, err := h.container.Content.ImportHSK(ctx, contentapp.ImportInput{Level: "HSK1"})
	require.NoError(t, err)
	require.Equal(t, 0, second.CardsAdded, "lần 2 phải báo cardsAdded = 0")
	require.Equal(t, 0, second.DictAdded, "lần 2 phải báo dictAdded = 0")
	require.Equal(t, first.CardsTotal, second.CardsTotal, "tổng số thẻ không đổi")
}

// `gradeTone` trả payload kèm `ok`, KHÔNG trả error từ resolver — để 1 query có
// nhiều alias vẫn giữ được phần thành công.
func Test_grade_tone_returns_ok_false_with_message_on_bad_input(t *testing.T) {
	h := newHarness(t)
	var resp struct {
		GradeTone struct {
			Ok    bool                      `json:"ok"`
			Grade *struct{ Grade, Hit int } `json:"grade"`
			Error *userError                `json:"error"`
		} `json:"gradeTone"`
	}
	h.client(t).MustPost(`query { gradeTone(expected: "không-phải-pinyin!!", answered: "x") { ok grade { grade hit } error { message code } } }`, &resp)

	require.False(t, resp.GradeTone.Ok)
	require.NotNil(t, resp.GradeTone.Error)
	require.Equal(t, "BAD_REQUEST", resp.GradeTone.Error.Code)
}

func Test_grade_tone_exact_match_grades_4(t *testing.T) {
	h := newHarness(t)
	var resp struct {
		GradeTone struct {
			Ok    bool `json:"ok"`
			Grade struct {
				Grade, Hit int
				Exact      bool `json:"exact"`
			} `json:"grade"`
		} `json:"gradeTone"`
	}
	h.client(t).MustPost(`query { gradeTone(expected: "ni3 hao3", answered: "ni3 hao3") { ok grade { grade hit exact } } }`, &resp)

	require.True(t, resp.GradeTone.Ok)
	require.Equal(t, 4, resp.GradeTone.Grade.Grade, "trùng hết = 4 (Dễ)")
	require.True(t, resp.GradeTone.Grade.Exact)
}

func Test_thieu_axes_returns_all_eight_axes_sorted(t *testing.T) {
	h := newHarness(t)
	var resp struct {
		Axes []struct{ Code, Name, Desc string } `json:"thieuAxes"`
	}
	h.client(t).MustPost(`query { thieuAxes { code name desc } }`, &resp)

	require.Len(t, resp.Axes, 8, "checklist THIEU có 8 trục A-H")
	for i, a := range resp.Axes {
		t.Run(fmt.Sprintf("axis %d", i), func(t *testing.T) {
			require.Equal(t, string(rune('A'+i)), a.Code, "trục phải ra theo thứ tự A..H ổn định")
			require.NotEmpty(t, a.Name)
		})
	}
}

func Test_append_thieu_rejects_incomplete_checklist(t *testing.T) {
	h := newHarness(t)
	var resp struct {
		AppendThieu struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"appendThieu"`
	}
	// Chỉ 2/8 trục: bản chấm thiếu trục không có nghĩa checklist "đánh giá buổi
	// luyện" ⇒ 400.
	h.client(t).MustPost(
		`mutation($s: [ThieuScoreInput!]!) { appendThieu(input: {scores: $s}) { ok error { message code } } }`,
		&resp, client.Var("s", []map[string]any{
			{"axis": "A", "value": 3}, {"axis": "B", "value": 4},
		}))

	require.False(t, resp.AppendThieu.Ok)
	require.Equal(t, "BAD_REQUEST", resp.AppendThieu.Error.Code)
}

// ── roadmap ─────────────────────────────────────────────────────────────────

func Test_set_stage_status_done_sets_completed_at_and_progress_rises(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedPathWithTopics(t, h, "zh", 2)
	path, err := h.container.Roadmap.PathBySlug(ctx, "zh")
	require.NoError(t, err)
	views, err := h.container.Roadmap.StageTreeByPathIDs(ctx, []int64{path.ID})
	require.NoError(t, err)
	stageID := views[path.ID][0].ID
	topicID := views[path.ID][0].Topics[0].ID

	var setTopic struct {
		SetTopicStatus struct {
			Ok    bool `json:"ok"`
			Topic struct {
				Status      string  `json:"status"`
				CompletedAt *string `json:"completedAt"`
			} `json:"topic"`
			Error *userError `json:"error"`
		} `json:"setTopicStatus"`
	}
	h.client(t).MustPost(
		`mutation($id: ID!) { setTopicStatus(id: $id, input: {status: DONE}) { ok topic { status completedAt } error { message code } } }`,
		&setTopic, client.Var("id", fmt.Sprint(topicID)))

	require.True(t, setTopic.SetTopicStatus.Ok, "lỗi: %+v", setTopic.SetTopicStatus.Error)
	require.Equal(t, "DONE", setTopic.SetTopicStatus.Topic.Status)
	require.NotNil(t, setTopic.SetTopicStatus.Topic.CompletedAt,
		"vào done phải set completed_at (amendment A1)")

	// Lần gọi `setStageStatus` với status DONE ở stage cha.
	var setStage struct {
		SetStageStatus struct {
			Ok    bool `json:"ok"`
			Stage struct {
				Status      string  `json:"status"`
				CompletedAt *string `json:"completedAt"`
			} `json:"stage"`
		} `json:"setStageStatus"`
	}
	h.client(t).MustPost(
		`mutation($id: ID!) { setStageStatus(id: $id, input: {status: DONE}) { ok stage { status completedAt } } }`,
		&setStage, client.Var("id", fmt.Sprint(stageID)))
	require.True(t, setStage.SetStageStatus.Ok)
	require.NotNil(t, setStage.SetStageStatus.Stage.CompletedAt)

	var prog struct {
		PathProgress struct {
			Ok       bool `json:"ok"`
			Progress struct {
				TopicsDone       int `json:"topicsDone"`
				Percent          int `json:"percent"`
				CompletedInRange int `json:"completedInRange"`
			} `json:"progress"`
			Error *userError `json:"error"`
		} `json:"pathProgress"`
	}
	h.client(t).MustPost(`query { pathProgress(slug: "zh") { ok progress { topicsDone percent completedInRange } error { message code } } }`, &prog)
	require.True(t, prog.PathProgress.Ok)
	require.Equal(t, 1, prog.PathProgress.Progress.TopicsDone)
	require.Equal(t, 50, prog.PathProgress.Progress.Percent, "1/2 topic bắt buộc = 50%")
}

func Test_path_progress_rejects_malformed_since(t *testing.T) {
	h := newHarness(t)
	seedPathWithTopics(t, h, "zh", 1)

	var resp struct {
		PathProgress struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"pathProgress"`
	}
	h.client(t).MustPost(
		`query($s: String) { pathProgress(slug: "zh", since: $s) { ok error { message code } } }`,
		&resp, client.Var("s", "28-09-2026"))

	require.False(t, resp.PathProgress.Ok)
	require.Equal(t, "BAD_REQUEST", resp.PathProgress.Error.Code,
		"since sai định dạng phải là 400, không phải âm thầm coi như hôm nay")
}

// Test paths list trả summary RIÊNG cho từng path — bắt hồi quy M4 ở tầng
// transport.
//
// Hồi quy: `ListPaths` gọi `ProgressByIDs` (gộp) nên mọi dòng hiện CÙNG 1 tổng.
// Test cùng tầng application (`Test_list_paths_gives_each_path_its_own_progress`)
// bắt được, nhưng chỉ qua `graph/client` mới chứng minh con số đó đi tới được
// client mà không bị view model làm tròn/sai.
// summaryValue là cặp (percent, topicsTotal, topicsDone) rút gọn để assert
// theo slug — struct ẩn danh trong map không gán được (Go không cho gán struct
// ẩn danh vào nhau khi khác kiểu khai báo).
type summaryValue struct {
	Percent     int
	TopicsTotal int
	TopicsDone  int
}

func Test_paths_list_returns_distinct_summary_per_path(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// 1 topic xong hết (100%) vs 3 topic chưa làm gì (0%). Số GỘP của hồi quy là
	// 1/4 = 25% cho cả hai — khác rõ 100 và 0.
	seedPathWithTopics(t, h, "xong-het", 1)
	seedPathWithTopics(t, h, "chua-lam-gi", 3)
	pathA, err := h.container.Roadmap.PathBySlug(ctx, "xong-het")
	require.NoError(t, err)
	views, err := h.container.Roadmap.StageTreeByPathIDs(ctx, []int64{pathA.ID})
	require.NoError(t, err)
	_, err = h.container.Roadmap.SetTopicStatus(ctx, views[pathA.ID][0].Topics[0].ID, ptr("done"), ptr(""))
	require.NoError(t, err)

	var resp struct {
		Paths []struct {
			Path struct {
				Slug string `json:"slug"`
			} `json:"path"`
			Summary struct {
				Percent     int `json:"percent"`
				TopicsTotal int `json:"topicsTotal"`
				TopicsDone  int `json:"topicsDone"`
			} `json:"summary"`
		} `json:"paths"`
	}
	h.client(t).MustPost(`query { paths { path { slug } summary { percent topicsTotal topicsDone } } }`, &resp)

	require.Len(t, resp.Paths, 2)
	bySlug := map[string]summaryValue{}
	for _, p := range resp.Paths {
		bySlug[p.Path.Slug] = summaryValue{p.Summary.Percent, p.Summary.TopicsTotal, p.Summary.TopicsDone}
	}
	require.Contains(t, bySlug, "xong-het")
	require.Contains(t, bySlug, "chua-lam-gi")
	require.Equal(t, 1, bySlug["xong-het"].TopicsTotal)
	require.Equal(t, 3, bySlug["chua-lam-gi"].TopicsTotal)
	require.Equal(t, 100, bySlug["xong-het"].Percent, "1/1 topic xong = 100%")
	require.Equal(t, 0, bySlug["chua-lam-gi"].Percent, "0/3 topic = 0%")
}

// `Path.progress` của query list cũng phải per-path (loader `progress`), không
// phải số gộp — cùng lý do, khác đường đọc.
func Test_paths_list_progress_field_is_per_path_not_aggregate(t *testing.T) {
	h := newHarness(t)
	seedPathWithTopics(t, h, "a", 1)
	seedPathWithTopics(t, h, "b", 3)

	var resp struct {
		Paths []struct {
			Path struct {
				Slug     string `json:"slug"`
				Progress struct {
					TopicsTotal int `json:"topicsTotal"`
				} `json:"progress"`
			} `json:"path"`
		} `json:"paths"`
	}
	h.client(t).MustPost(`query { paths { path { slug progress { topicsTotal } } } }`, &resp)

	require.Len(t, resp.Paths, 2)
	bySlug := map[string]int{}
	for _, p := range resp.Paths {
		bySlug[p.Path.Slug] = p.Path.Progress.TopicsTotal
	}
	require.Equal(t, 1, bySlug["a"], "path a có 1 topic")
	require.Equal(t, 3, bySlug["b"], "path b có 3 topic")
	require.NotEqual(t, 4, bySlug["a"], "4 là tổng của cả 2 path — dấu hiệu số GỘP")
}

func Test_create_path_duplicate_slug_returns_409(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, err := h.container.Roadmap.CreatePath(ctx, roadmapapp.PathInput{Slug: "zh", Title: "A"})
	require.NoError(t, err)

	var resp struct {
		CreatePath struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"createPath"`
	}
	h.client(t).MustPost(
		`mutation { createPath(input: {slug: "zh", title: "B"}) { ok error { message code } } }`, &resp)

	require.False(t, resp.CreatePath.Ok)
	require.Equal(t, "CONFLICT", resp.CreatePath.Error.Code)
}

func Test_delete_path_soft_deletes_cascade_and_then_404(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedPathWithTopics(t, h, "zh", 2)

	var resp struct {
		DeletePath struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"deletePath"`
	}
	h.client(t).MustPost(`mutation { deletePath(slug: "zh") { ok error { message code } } }`, &resp)
	require.True(t, resp.DeletePath.Ok, "lỗi: %+v", resp.DeletePath.Error)

	// Cây phải biến mất (soft delete + cascade trong cùng transaction).
	_, err := h.container.Roadmap.PathTree(ctx, "zh")
	require.Error(t, err)

	// Xoá lần 2 phải 404, không phải 200 im lặng.
	var again struct {
		DeletePath struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"deletePath"`
	}
	h.client(t).MustPost(`mutation { deletePath(slug: "zh") { ok error { message code } } }`, &again)
	require.False(t, again.DeletePath.Ok)
	require.Equal(t, "NOT_FOUND", again.DeletePath.Error.Code)
}

// Node đã xoá mềm phải trả `null` chứ không phải object rỗng — client phân biệt
// "không có" với "có nhưng rỗng".
func Test_path_returns_null_for_unknown_slug(t *testing.T) {
	h := newHarness(t)
	var resp struct {
		Path *struct{ ID string } `json:"path"`
	}
	h.client(t).MustPost(`query { path(slug: "khong-co") { id } }`, &resp)

	require.Nil(t, resp.Path, "path không tồn tại phải trả null, không phải lỗi")
}

// ── practice ────────────────────────────────────────────────────────────────

func Test_diff_scores_transcript_and_lists_wrong_words(t *testing.T) {
	h := newHarness(t)
	var resp struct {
		Diff struct {
			Ok     bool `json:"ok"`
			Result struct {
				Transcript string   `json:"transcript"`
				Wrong      []string `json:"wrong"`
				Score      float64  `json:"score"`
				Diff       []struct {
					Text, Status string
				} `json:"diff"`
			} `json:"result"`
			Error *userError `json:"error"`
		} `json:"diff"`
	}
	h.client(t).MustPost(
		`query($s: String!, $t: String!) { diff(sample: $s, transcript: $t) { ok result { transcript wrong score diff { text status } } error { message code } } }`,
		&resp, client.Var("s", "ni hao"), client.Var("t", "ni hao"))

	require.True(t, resp.Diff.Ok, "lỗi: %+v", resp.Diff.Error)
	require.Equal(t, 1.0, resp.Diff.Result.Score)
	require.Empty(t, resp.Diff.Result.Wrong)
	require.NotEmpty(t, resp.Diff.Result.Diff)
}

func Test_diff_normalizes_traditional_to_simplified(t *testing.T) {
	h := newHarness(t)
	var resp struct {
		Diff struct {
			Ok     bool `json:"ok"`
			Result struct {
				Transcript string   `json:"transcript"`
				Wrong      []string `json:"wrong"`
			} `json:"result"`
		} `json:"diff"`
	}
	// Whisper trả chữ phồn "學習" còn deck mẫu dùng giản "学习" — không chuẩn hoá
	// thì chấm sai oan dù đọc đúng.
	h.client(t).MustPost(
		`query { diff(sample: "学习", transcript: "學習") { ok result { transcript wrong } } }`, &resp)

	require.Equal(t, "学习", resp.Diff.Result.Transcript,
		"transcript phải được chuẩn hoá phồn→giản trước khi so")
	require.Empty(t, resp.Diff.Result.Wrong, "đọc đúng chữ nghĩa thì không có từ sai")
}

func Test_shadow_progress_returns_zero_state_for_never_practised_card(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	deck, err := h.container.SRS.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err)
	card, err := h.container.SRS.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "你", Back: "bạn"})
	require.NoError(t, err)

	var resp struct {
		ShadowProgress struct {
			Loops  int     `json:"loops"`
			Rate   float64 `json:"rate"`
			CardID string  `json:"cardId"`
		} `json:"shadowProgress"`
	}
	h.client(t).MustPost(`query($c: ID!) { shadowProgress(cardId: $c) { loops rate cardId } }`, &resp,
		client.Var("c", fmt.Sprint(card.ID)))

	require.Equal(t, 0, resp.ShadowProgress.Loops, "chưa luyện lần nào KHÔNG phải 404")
	require.Equal(t, 1.0, resp.ShadowProgress.Rate, "rate mặc định 1.0")
}

func Test_record_shadow_progress_rejects_rate_out_of_range(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	deck, err := h.container.SRS.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err)
	card, err := h.container.SRS.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "你", Back: "bạn"})
	require.NoError(t, err)

	var resp struct {
		RecordShadowProgress struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"recordShadowProgress"`
	}
	h.client(t).MustPost(
		`mutation($c: ID!) { recordShadowProgress(input: {cardId: $c, loops: 3, rate: 3.0}) { ok error { message code } } }`,
		&resp, client.Var("c", fmt.Sprint(card.ID)))

	require.False(t, resp.RecordShadowProgress.Ok)
	require.Equal(t, "BAD_REQUEST", resp.RecordShadowProgress.Error.Code,
		"rate ngoài [0.5, 1.5] phải 400 (NormalizeRate)")
}

func Test_append_error_and_list_errors_round_trip(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	entry, err := h.container.Practice.AppendError(ctx, nil, "学习", "学习", []string{"学"})
	require.NoError(t, err)
	require.NotZero(t, entry.ID)

	var resp struct {
		Errors []struct {
			ID       string   `json:"id"`
			Expected string   `json:"expected"`
			Wrong    []string `json:"wrong"`
			CardID   *string  `json:"cardId"`
		} `json:"errors"`
	}
	h.client(t).MustPost(`query { errors { id expected wrong cardId } }`, &resp)

	require.NotEmpty(t, resp.Errors)
	require.Equal(t, "学习", resp.Errors[0].Expected)
	require.Equal(t, []string{"学"}, resp.Errors[0].Wrong)
	require.Nil(t, resp.Errors[0].CardID, "lỗi luyện tự do không gắn thẻ")
}

func Test_mark_error_resolved_returns_new_note_id(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	entry, err := h.container.Practice.AppendError(ctx, nil, "学习", "学习", []string{"学"})
	require.NoError(t, err)

	var resp struct {
		MarkErrorResolved struct {
			Ok     bool       `json:"ok"`
			NoteID string     `json:"noteId"`
			Error  *userError `json:"error"`
		} `json:"markErrorResolved"`
	}
	h.client(t).MustPost(`mutation($id: ID!) { markErrorResolved(errorId: $id) { ok noteId error { message code } } }`,
		&resp, client.Var("id", fmt.Sprint(entry.ID)))

	require.True(t, resp.MarkErrorResolved.Ok, "lỗi: %+v", resp.MarkErrorResolved.Error)
	require.NotEmpty(t, resp.MarkErrorResolved.NoteID)
	require.NotEqual(t, fmt.Sprint(entry.ID), resp.MarkErrorResolved.NoteID,
		"sổ lỗi append-only nên phải ghi note MỚI, không sửa note gốc")
}

// ── insight ─────────────────────────────────────────────────────────────────

func Test_stats_reports_utc_timezone_and_known_numbers(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	deck, err := h.container.SRS.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err)
	card, err := h.container.SRS.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "你", Back: "bạn"})
	require.NoError(t, err)
	_, err = h.container.SRS.RecordReview(ctx, srsapp.ReviewInput{CardID: card.ID, Grade: 4})
	require.NoError(t, err)

	var resp struct {
		Stats struct {
			Ok    bool `json:"ok"`
			Stats struct {
				Range, Timezone string
				TotalAll        int     `json:"totalAll"`
				DueNow          int     `json:"dueNow"`
				DoneWindow      int     `json:"doneWindow"`
				Accuracy        float64 `json:"accuracy"`
			} `json:"stats"`
			Error *userError `json:"error"`
		} `json:"stats"`
	}
	h.client(t).MustPost(`query { stats(range: "week") { ok stats { range timezone totalAll dueNow doneWindow accuracy } error { message code } } }`, &resp)

	require.True(t, resp.Stats.Ok, "lỗi: %+v", resp.Stats.Error)
	require.Equal(t, "UTC", resp.Stats.Stats.Timezone, "hợp đồng đóng băng: mọi mốc là UTC")
	require.Equal(t, "week", resp.Stats.Stats.Range)
	require.Equal(t, 1, resp.Stats.Stats.TotalAll)
	require.Equal(t, 1, resp.Stats.Stats.DoneWindow, "vừa ôn 1 lần trong tuần")
	require.Equal(t, 0, resp.Stats.Stats.DueNow, "thẻ vừa ôn còn 1 ngày nên chưa tới hạn")
	require.InDelta(t, 1.0, resp.Stats.Stats.Accuracy, 0.001, "grade 4 = đạt")
}

func Test_progress_over_range_counts_completed_topics(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedPathWithTopics(t, h, "zh", 2)
	path, err := h.container.Roadmap.PathBySlug(ctx, "zh")
	require.NoError(t, err)
	views, err := h.container.Roadmap.StageTreeByPathIDs(ctx, []int64{path.ID})
	require.NoError(t, err)
	tv := views[path.ID][0].Topics[0]
	_, err = h.container.Roadmap.SetTopicStatus(ctx, tv.ID, ptr("done"), ptr(""))
	require.NoError(t, err)

	var resp struct {
		ProgressOverRange struct {
			Ok     bool `json:"ok"`
			Result struct {
				Since     string `json:"since"`
				Completed int    `json:"completed"`
			} `json:"result"`
			Error *userError `json:"error"`
		} `json:"progressOverRange"`
	}
	h.client(t).MustPost(`query($s: String!) { progressOverRange(since: $s) { ok result { since completed } error { message code } } }`,
		&resp, client.Var("s", "2000-01-01"))

	require.True(t, resp.ProgressOverRange.Ok, "lỗi: %+v", resp.ProgressOverRange.Error)
	require.Equal(t, "2000-01-01", resp.ProgressOverRange.Result.Since, "since phải chuẩn hoá")
	require.Equal(t, 1, resp.ProgressOverRange.Result.Completed)
}

func ptr[T any](v T) *T { return &v }
