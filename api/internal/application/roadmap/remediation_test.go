package roadmap

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Các test dưới đây chốt lỗi của cổng Oracle M2 ở TẦNG APPLICATION. Trước
// đó chỉ có assert trong DB nên lỗi P0 (ghi đúng DB nhưng trả về row CŨ) lọt
// qua toàn bộ suite — assert GIÁ TRỊ TRẢ VỀ là thứ phát hiện được.

// ── F1: giá trị trả về phải là giá trị vừa ghi ──────────────────────────────

func Test_update_topic_returns_the_value_just_written(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)
	tp, err := svc.CreateTopic(ctx, stage.ID, TopicInput{Title: "T ban đầu"})
	require.NoError(t, err)

	got, err := svc.UpdateTopic(ctx, tp.ID, TopicPatch{
		Title: strPtr("T đã đổi"), Why: strPtr("vì sao mới"), Position: intPtr(7),
	})
	require.NoError(t, err)
	assert.Equal(t, "T đã đổi", got.Title, "trả về title cũ là đọc ngoài transaction")
	assert.Equal(t, "vì sao mới", got.Why)
	assert.Equal(t, 7, got.Position)
}

func Test_update_stage_returns_the_value_just_written(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G cũ", DurationWeeks: intPtr(1)})
	require.NoError(t, err)

	got, err := svc.UpdateStage(ctx, stage.ID, StagePatch{
		Title: strPtr("G mới"), Goal: strPtr("mục tiêu mới"), DurationWeeks: intPtr(9),
	})
	require.NoError(t, err)
	assert.Equal(t, "G mới", got.Title)
	assert.Equal(t, "mục tiêu mới", got.Goal)
	assert.Equal(t, 9, got.DurationWeeks)
}

func Test_update_path_returns_the_value_just_written(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "Tiêu đề cũ"})
	require.NoError(t, err)

	got, err := svc.UpdatePath(ctx, "p", PathPatch{
		Title: strPtr("Tiêu đề mới"), Overview: strPtr("mô tả mới"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Tiêu đề mới", got.Title)
	assert.Equal(t, "mô tả mới", got.Overview)
}

func Test_update_resource_and_milestone_return_the_value_just_written(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)
	tp, err := svc.CreateTopic(ctx, stage.ID, TopicInput{Title: "T"})
	require.NoError(t, err)

	res, err := svc.CreateResource(ctx, tp.ID, ResourceInput{Title: "R cũ", Kind: "video"})
	require.NoError(t, err)
	gotRes, err := svc.UpdateResource(ctx, res.ID, ResourcePatch{
		Title: strPtr("R mới"), Note: strPtr("ghi chú"), Position: intPtr(3),
	})
	require.NoError(t, err)
	assert.Equal(t, "R mới", gotRes.Title)
	assert.Equal(t, "ghi chú", gotRes.Note)
	assert.Equal(t, 3, gotRes.Position)

	ms, err := svc.CreateMilestone(ctx, stage.ID, MilestoneInput{Text: "M cũ"})
	require.NoError(t, err)
	gotMs, err := svc.UpdateMilestone(ctx, ms.ID, MilestonePatch{
		Text: strPtr("M mới"), Position: intPtr(4),
	})
	require.NoError(t, err)
	assert.Equal(t, "M mới", gotMs.Text)
	assert.Equal(t, 4, gotMs.Position)
}

func Test_set_topic_status_returns_the_status_just_written(t *testing.T) {
	svc, repo, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)
	tp, err := svc.CreateTopic(ctx, stage.ID, TopicInput{Title: "T"})
	require.NoError(t, err)

	got, err := svc.SetTopicStatus(ctx, tp.ID, strPtr(StatusDone), strPtr("xong rồi"))
	require.NoError(t, err)
	assert.Equal(t, StatusDone, got.Status, "trả về not_started là đọc ngoài transaction")
	assert.Equal(t, "xong rồi", got.StatusNote)
	require.NotNil(t, got.CompletedAt)
	assert.Equal(t, fixedClock.Format(time.RFC3339), *got.CompletedAt)
	// Row trong repo (tức DB) cũng phải khớp — hai bên không được lệch.
	assert.Equal(t, StatusDone, repo.topics[tp.ID].Status)
	assert.Equal(t, "xong rồi", repo.topics[tp.ID].StatusNote)
}

func Test_set_stage_status_returns_the_status_just_written(t *testing.T) {
	svc, repo, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)

	got, err := svc.SetStageStatus(ctx, stage.ID, strPtr(StatusSkipped), strPtr("bỏ qua"))
	require.NoError(t, err)
	assert.Equal(t, StatusSkipped, got.Status)
	assert.Equal(t, "bỏ qua", got.StatusNote)
	assert.Nil(t, got.CompletedAt, "skipped không phải done thì không có mốc")
	assert.Equal(t, StatusSkipped, repo.stages[stage.ID].Status)
}

func Test_set_status_leaving_done_clears_completed_at_in_returned_value(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)
	tp, err := svc.CreateTopic(ctx, stage.ID, TopicInput{Title: "T"})
	require.NoError(t, err)

	_, err = svc.SetTopicStatus(ctx, tp.ID, strPtr(StatusDone), nil)
	require.NoError(t, err)
	got, err := svc.SetTopicStatus(ctx, tp.ID, strPtr(StatusInProgress), nil)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status)
	assert.Nil(t, got.CompletedAt, "rời done phải clear thành NULL trong giá trị trả về")
}

// ── F3: ClearMap xoá được toạ độ (NULL thật) ───────────────────────────────

func Test_clear_map_resets_node_to_auto_layout(t *testing.T) {
	svc, repo, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)
	tp, err := svc.CreateTopic(ctx, stage.ID, TopicInput{
		Title: "T", MapX: f64Ptr(123), MapY: f64Ptr(456),
	})
	require.NoError(t, err)
	require.NotNil(t, tp.MapX)

	got, err := svc.UpdateTopic(ctx, tp.ID, TopicPatch{ClearMap: true})
	require.NoError(t, err)
	assert.Nil(t, got.MapX, "ClearMap phải trả về NULL, không phải 0")
	assert.Nil(t, got.MapY)
	assert.Nil(t, repo.topics[tp.ID].MapX, "row trong DB phải NULL")
	assert.Nil(t, repo.topics[tp.ID].MapY)
}

func Test_clear_map_alone_is_a_valid_patch(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)
	tp, err := svc.CreateTopic(ctx, stage.ID, TopicInput{Title: "T", MapX: f64Ptr(1)})
	require.NoError(t, err)

	// Patch CHỈ có ClearMap phải được chấp nhận — trước đây rơi vào nhánh
	// "không có gì để cập nhật" vì mọi field đều nil.
	_, err = svc.UpdateTopic(ctx, tp.ID, TopicPatch{ClearMap: true})
	require.NoError(t, err)
}

// ── F7: deck.lang phải khớp path.language; GetPath trả deck cho M6 ───────────

func Test_create_stage_rejects_deck_with_mismatched_language(t *testing.T) {
	repo := newFakeRepo()
	decks := fakeDecks{exists: map[int64]string{7: "en"}}
	svc := NewService(repo, &fakeUow{repo: repo}, decks, func() time.Time { return fixedClock }, ViewBox{})
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P", Language: "zh"})
	require.NoError(t, err)

	enDeck := int64(7)
	_, err = svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G", DeckID: &enDeck})
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
	assert.Contains(t, appErr.Message, "không khớp")
	assert.Empty(t, repo.stages, "stage lệch ngôn ngữ không được ghi")
}

func Test_update_stage_rejects_deck_with_mismatched_language(t *testing.T) {
	repo := newFakeRepo()
	decks := fakeDecks{exists: map[int64]string{7: "en"}}
	svc := NewService(repo, &fakeUow{repo: repo}, decks, func() time.Time { return fixedClock }, ViewBox{})
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P", Language: "zh"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)

	enDeck := int64(7)
	_, err = svc.UpdateStage(ctx, stage.ID, StagePatch{DeckID: &enDeck})
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
	assert.Nil(t, repo.stages[stage.ID].DeckID, "deck lệch ngôn ngữ không được ghi")
}

func Test_deck_with_free_form_path_language_is_allowed(t *testing.T) {
	repo := newFakeRepo()
	decks := fakeDecks{exists: map[int64]string{7: "zh"}}
	svc := NewService(repo, &fakeUow{repo: repo}, decks, func() time.Time { return fixedClock }, ViewBox{})
	ctx := context.Background()
	// Path ngôn ngữ tự do ("vi") không nằm trong tập đóng băng {zh, en} nên
	// KHÔNG được chặn oan — `path.language` là chuỗi tự do tối đa 16 ký tự.
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P", Language: "vi"})
	require.NoError(t, err)
	zhDeck := int64(7)
	stage, err := svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G", DeckID: &zhDeck})
	require.NoError(t, err)
	require.NotNil(t, stage.DeckID)
}

func Test_get_path_exposes_deck_name_and_lang_for_review_button(t *testing.T) {
	repo := newFakeRepo()
	decks := fakeDecks{exists: map[int64]string{7: "zh"}}
	svc := NewService(repo, &fakeUow{repo: repo}, decks, func() time.Time { return fixedClock }, ViewBox{})
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P", Language: "zh"})
	require.NoError(t, err)
	zhDeck := int64(7)
	_, err = svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G", DeckID: &zhDeck})
	require.NoError(t, err)

	view, err := svc.GetPath(ctx, "p")
	require.NoError(t, err)
	require.Len(t, view.Stages, 1)
	require.NotNil(t, view.Stages[0].Deck, "M6 cần deck_name/deck_lang để hiện nút 'vào /review'")
	assert.Equal(t, int64(7), view.Stages[0].Deck.ID)
	assert.Equal(t, "zh", view.Stages[0].Deck.Lang)
	assert.NotEmpty(t, view.Stages[0].Deck.Name, "client hiện tên deck, không hiện id")
}

func Test_get_path_stage_without_deck_leaves_ref_nil(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreatePath(ctx, PathInput{Slug: "p", Title: "P"})
	require.NoError(t, err)
	_, err = svc.CreateStage(ctx, "p", StageInput{Slug: "g", Title: "G"})
	require.NoError(t, err)

	view, err := svc.GetPath(ctx, "p")
	require.NoError(t, err)
	require.Len(t, view.Stages, 1)
	assert.Nil(t, view.Stages[0].Deck)
}
