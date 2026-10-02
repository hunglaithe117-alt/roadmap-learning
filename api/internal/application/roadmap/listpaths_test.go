package roadmap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// seedPath tạo 1 path + `stages` stage, mỗi stage `topicsPerStage` topic.
//
// `Position` truyền TƯỜNG (1, 2, 3…) chứ không để `CreateStage` tự tính: fake
// `MaxStagePosition` trả cứng -1 nên mọi stage đều có `position = 0`. Với
// position bằng nhau, test thứ tự chỉ còn chạy nhánh tiebreak theo ID — tức
// không kiểm đúng thứ production sắp theo. Truyền tường thì test kiểm đúng
// `ORDER BY position`.
func seedPath(t *testing.T, s *Service, slug string, stages, topicsPerStage int) {
	t.Helper()
	ctx := context.Background()
	_, err := s.CreatePath(ctx, PathInput{Slug: slug, Title: "path " + slug, Language: "zh"})
	require.NoError(t, err)
	for i := 0; i < stages; i++ {
		pos := i + 1
		stage, err := s.CreateStage(ctx, slug, StageInput{
			Slug: slug + "-s" + itoa(i), Title: "giai doan " + itoa(i), Position: &pos,
		})
		require.NoError(t, err)
		for j := 0; j < topicsPerStage; j++ {
			_, err := s.CreateTopic(ctx, stage.ID, TopicInput{Title: "chu de " + itoa(j)})
			require.NoError(t, err)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// Test_list_paths_gives_each_path_its_own_progress — chặn hồi quy M4.
//
// Hồi quy: `ListPaths` gọi `ProgressByIDs` (GỘP toàn bộ path thành 1 con số) rồi
// gán cùng số đó cho mọi dòng. Test này không bắt được khi chỉ có 1 path, vì
// khi đó aggregate = per-path.
//
// Dữ liệu chọn CỐ Ý để hồi quy tạo ra kết quả KHÁC HẲN chứ không chỉ khác
// một chút:
//
//	path A: 1 topic, đánh dấu xong  → 1/1 = 100%
//	path B: 3 topic, chưa xong cái nào → 0/3 = 0%
//	SỐ GỘP (hồi quy): 1/4 = 25% cho CẢ HAI dòng
//
// Nếu 2 path chỉ khác `topicsTotal` mà cùng 0% thì `NotEqual(percent)` không bắt
// được gì — đó là lý do test cần khác nhau ở CẢ mẫu số lẫn tử số.
//
// Đây là test thiếu suốt từ M2 (test `ListPaths` duy nhất dùng 1 path nên vô tình
// đúng), và là lý do hồi quy lọt qua được cả gate M4.
func Test_list_paths_gives_each_path_its_own_progress(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, nil, nil, nil, ViewBox{})
	ctx := context.Background()

	seedPath(t, svc, "xong-het", 1, 1)
	seedPath(t, svc, "chua-lam-gi", 1, 3)

	// Đánh dấu topic đầu của path "xong-het" là xong.
	a := repo.pathBySlug(t, "xong-het")
	topicA := repo.firstTopicOf(t, a.ID)
	_, err := svc.SetTopicStatus(ctx, topicA, ptrStr("done"), ptrStr(""))
	require.NoError(t, err)

	got, err := svc.ListPaths(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)

	bySlug := map[string]Summary{}
	for _, s := range got {
		bySlug[s.Slug] = s.Progress
	}
	require.Equal(t, 1, bySlug["xong-het"].TopicsTotal, "path nhỏ phải có đúng 1 topic")
	require.Equal(t, 3, bySlug["chua-lam-gi"].TopicsTotal, "path lớn phải có đúng 3 topic")
	require.Equal(t, 1, bySlug["xong-het"].TopicsDone)
	require.Equal(t, 0, bySlug["chua-lam-gi"].TopicsDone)
	require.Equal(t, 100, bySlug["xong-het"].Percent)
	require.Equal(t, 0, bySlug["chua-lam-gi"].Percent)
	// Số GỘP của hồi quy là 25% cho cả hai — khác rõ 100 và 0.
	require.NotEqual(t, 25, bySlug["xong-het"].Percent,
		"25% là số GỘP của hồi quy (1/4); thấy nó nghĩa là vẫn đang gộp mọi path")
}

func ptrStr(v string) *string { return &v }

// pathBySlug tra path theo slug trong fakeRepo (test cần id để sửa topic).
func (f *fakeRepo) pathBySlug(t *testing.T, slug string) Path {
	t.Helper()
	p, err := f.PathBySlug(context.Background(), slug)
	require.NoError(t, err)
	return p
}

// firstTopicOf tra id topic đầu tiên của path.
func (f *fakeRepo) firstTopicOf(t *testing.T, pathID int64) int64 {
	t.Helper()
	for _, s := range f.stages {
		if s.PathID != pathID {
			continue
		}
		for topicID, tp := range f.topics {
			if tp.StageID == s.ID {
				return topicID
			}
		}
	}
	t.Fatalf("path %d không có topic nào", pathID)
	return 0
}

// `topics_optional` không nằm trong `Summary` (đó là `Progress` đầy đủ) nhưng
// `topics_required` thì có — path có topic optional phải có mẫu số nhỏ hơn
// `topics_total`, và việc đó phải đúng RIÊNG cho từng path.
func Test_list_paths_required_excludes_optional_per_path(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, nil, nil, nil, ViewBox{})
	ctx := context.Background()

	_, err := svc.CreatePath(ctx, PathInput{Slug: "a", Title: "A"})
	require.NoError(t, err)
	stage, err := svc.CreateStage(ctx, "a", StageInput{Slug: "s1", Title: "S1"})
	require.NoError(t, err)
	_, err = svc.CreateTopic(ctx, stage.ID, TopicInput{Title: "bat buoc"})
	require.NoError(t, err)
	optional := 1
	_, err = svc.CreateTopic(ctx, stage.ID, TopicInput{Title: "tham khao", IsOptional: &optional})
	require.NoError(t, err)

	_, err = svc.CreatePath(ctx, PathInput{Slug: "b", Title: "B"})
	require.NoError(t, err)
	stageB, err := svc.CreateStage(ctx, "b", StageInput{Slug: "s1", Title: "S1"})
	require.NoError(t, err)
	for i := 0; i < 4; i++ {
		_, err = svc.CreateTopic(ctx, stageB.ID, TopicInput{Title: "chu de " + itoa(i)})
		require.NoError(t, err)
	}

	got, err := svc.ListPaths(ctx)
	require.NoError(t, err)
	bySlug := map[string]Summary{}
	for _, s := range got {
		bySlug[s.Slug] = s.Progress
	}
	require.Equal(t, 2, bySlug["a"].TopicsTotal)
	require.Equal(t, 1, bySlug["a"].TopicsRequired, "topic optional không nằm trong mẫu số (A1)")
	require.Equal(t, 4, bySlug["b"].TopicsTotal)
	require.Equal(t, 4, bySlug["b"].TopicsRequired)
}

// Path KHÔNG có stage vẫn phải xuất hiện trong list với số 0, không biến mất
// (`ProgressByPathIDs` luôn điền key cho mọi id).
func Test_list_paths_includes_path_without_stage_with_zero_progress(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, nil, nil, nil, ViewBox{})
	ctx := context.Background()

	seedPath(t, svc, "co-noi-dung", 1, 2)
	_, err := svc.CreatePath(ctx, PathInput{Slug: "rong", Title: "Rong"})
	require.NoError(t, err)

	got, err := svc.ListPaths(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2, "path không có stage vẫn phải có trong danh sách")

	bySlug := map[string]Summary{}
	for _, s := range got {
		bySlug[s.Slug] = s.Progress
	}
	require.Equal(t, 0, bySlug["rong"].TopicsTotal)
	require.Equal(t, 0, bySlug["rong"].Percent)
	require.Equal(t, 2, bySlug["co-noi-dung"].TopicsTotal)
}

// `Path.progress` của query list phải dùng CÙNG hình dạng per-path — nếu ai đó
// đổi dataloader `progress` sang `ProgressByIDs` thì test này đỏ.
func Test_progress_by_path_ids_matches_single_path_progress_by_ids(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, nil, nil, nil, ViewBox{})
	ctx := context.Background()

	seedPath(t, svc, "nho", 1, 1)
	seedPath(t, svc, "lon", 2, 2)

	perPath, err := svc.ProgressByPathIDs(ctx, []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, perPath, 2)

	// `ProgressByIDs` với 1 path phải cho ĐÚNG kết quả per-path của path đó.
	for id, want := range perPath {
		got, err := svc.ProgressByIDs(ctx, []int64{id}, nil)
		require.NoError(t, err)
		require.Equal(t, want.TopicsTotal, got.TopicsTotal, "path %d: TopicsTotal lệch", id)
		require.Equal(t, want.Percent, got.Percent, "path %d: Percent lệch", id)
	}
}

// Test thứ tự stage trong cây phải ỔN ĐỊNH — bắt lỗi duyệt map.
//
// `StageTreeByPathIDs` gom stage theo path từ `StageTreeByIDs`, trả về map khoá
// stageID. Gom bằng `for _, views := range byStage` tức duyệt MAP, mà Go
// randomize thứ tự duyệt map ⇒ `path.stages` ra thứ tự NGẪU NHIÊN mỗi request.
// Cây roadmap vẽ theo `position` nên node nhảy lung tung trên bản đồ, và
// "topic đầu mỗi stage = CURRENT" áp vào sai topic — mà KHÔNG lỗi nào trả về.
//
// M4 không bắt được vì seed lúc đó chỉ có 1 stage: 1 phần tử thì thứ tự không
// thể sai. Test này seed N stage và gọi lặp nhiều lần trong cùng process —
// randomize của Go đủ để lộ trong vài chục lần gọi.
func Test_stage_order_in_tree_is_stable_across_calls(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, nil, nil, nil, ViewBox{})
	ctx := context.Background()

	seedPath(t, svc, "nhieu-stage", 6, 1)
	pathID := repo.pathBySlug(t, "nhieu-stage").ID

	want := []string{"1", "2", "3", "4", "5", "6"} // `position` tăng dần
	for round := 0; round < 50; round++ {
		byPath, err := svc.StageTreeByPathIDs(ctx, []int64{pathID})
		require.NoError(t, err)
		got := make([]string, 0, len(byPath[pathID]))
		for _, sv := range byPath[pathID] {
			got = append(got, itoa(sv.Position))
		}
		require.Equal(t, want, got,
			"lần %d: thứ tự stage trong cây bị xáo — đường đọc đang duyệt map", round)
	}
}

// `GetPath` và `StageTreeByPathIDs` là HAI đường đọc cây; chúng phải cho CÙNG
// thứ tự stage, nếu không client thấy cây đổi chỗ tuỳ đường gọi.
func Test_get_path_and_stage_tree_agree_on_stage_order(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, nil, nil, nil, ViewBox{})
	ctx := context.Background()

	seedPath(t, svc, "hai-duong", 4, 1)

	view, err := svc.GetPath(ctx, "hai-duong")
	require.NoError(t, err)
	pathID := repo.pathBySlug(t, "hai-duong").ID
	byPath, err := svc.StageTreeByPathIDs(ctx, []int64{pathID})
	require.NoError(t, err)
	require.Len(t, view.Stages, 4)
	require.Len(t, byPath[pathID], 4)
	for i := range view.Stages {
		require.Equal(t, view.Stages[i].ID, byPath[pathID][i].ID,
			"stage thứ %d lệch giữa GetPath và StageTreeByPathIDs", i)
	}
}
