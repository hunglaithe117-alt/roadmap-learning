package graphql_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	roadmapapp "langapp/internal/application/roadmap"
	domainroadmap "langapp/internal/domain/roadmap"
	platformtestdb "langapp/internal/platform/testdb"
)

// treeQuery là cây roadmap 5 tầng trong 1 request:
//
//	path → stages → topics → resources, cộng milestones và deck.
//
// Đây là cây query mà M6 cần để vẽ bản đồ: không có nó thì phải gọi 4 endpoint
// tuần tự, và mỗi node lại cần thêm 1 request cho resource.
const treeQuery = `query Tree($slug: String!) {
  path(slug: $slug) {
    id
    guid
    slug
    title
    progress { percent topicsTotal topicsDone }
    stages {
      id
      guid
      title
      position
      terrain
      direction
      deck { id name lang }
      milestones { id guid text }
      topics {
        id
        guid
        title
        isOptional
        mapPinned
        level
        point { x y }
        resources { id guid title kind }
      }
    }
  }
}`

// seedPathWithTopics tạo 1 path + 1 stage + `n` topic, mỗi topic 2 resource.
func seedPathWithTopics(t *testing.T, h *harness, slug string, n int) {
	t.Helper()
	ctx := context.Background()
	p, err := h.container.Roadmap.CreatePath(ctx, roadmapapp.PathInput{
		Slug: slug, Title: "path " + slug, Language: "zh",
	})
	require.NoError(t, err)
	st, err := h.container.Roadmap.CreateStage(ctx, slug, roadmapapp.StageInput{
		Slug: "g1", Title: "giai doan 1",
	})
	require.NoError(t, err)
	require.NotZero(t, p.ID)
	require.NotZero(t, st.ID)
	for i := 0; i < n; i++ {
		tp, err := h.container.Roadmap.CreateTopic(ctx, st.ID, roadmapapp.TopicInput{
			Title: fmt.Sprintf("chu de %d", i),
		})
		require.NoError(t, err)
		for j := 0; j < 2; j++ {
			_, err := h.container.Roadmap.CreateResource(ctx, tp.ID, roadmapapp.ResourceInput{
				Title: "tai lieu",
				Kind:  "video",
			})
			require.NoError(t, err)
		}
	}
}

// Test cây roadmap 5 tầng trả đủ trong 1 request.
//
// Yêu cầu M4 exit: "query cây roadmap 1 request trả đủ 5 tầng". Test này khẳng
// định TẤT CẢ 5 tầng có dữ liệu, không chỉ "không lỗi" — 1 cây trả rỗng ở tầng
// dưới cũng qua được `MustPost`.
func Test_roadmap_tree_query_returns_all_five_levels_in_one_request(t *testing.T) {
	h := newHarness(t)
	seedPathWithTopics(t, h, "zh", 3)

	var resp struct {
		Path treePathResp `json:"path"`
	}
	h.client(t).MustPost(treeQuery, &resp, client.Var("slug", "zh"))

	// Tầng 1: path.
	require.NotEmpty(t, resp.Path.ID)
	require.NotEmpty(t, resp.Path.GUID, "mọi type phải có guid để client định danh")
	require.Equal(t, "zh", resp.Path.Slug)
	require.Equal(t, 3, resp.Path.Progress.TopicsTotal)
	// 3 topic đều `not_started` ⇒ 0% — `ComputeProgress` chia trên tổng topic
	// BẮT BUỘC, mẫu số là 3 nên 0/3 = 0, không phải "chưa làm gì nên 100".
	require.Equal(t, 0, resp.Path.Progress.Percent)
	require.Equal(t, 0, resp.Path.Progress.TopicsDone)

	// Tầng 2: stage.
	require.Len(t, resp.Path.Stages, 1)
	stage := resp.Path.Stages[0]
	require.NotEmpty(t, stage.GUID)
	require.Equal(t, "MEADOW", stage.Terrain, "DEFAULT của cột terrain là 'meadow' (migration 00004)")
	require.Equal(t, "UP", stage.Direction, "DEFAULT của cột direction")
	require.Nil(t, stage.Deck, "stage chưa gắn deck")

	// Tầng 3: topic + layout + LevelState.
	require.Len(t, stage.Topics, 3)
	require.Equal(t, "CURRENT", stage.Topics[0].Level, "node đầu tiên luôn CURRENT")
	require.Equal(t, "LOCKED", stage.Topics[1].Level, "node sau node chưa xong phải LOCKED")
	// Layout `direction=UP` + `terrain=MEADOW` (amp = 40) ⇒ node nằm quanh
	// trục giữa ngang và đi từ ĐÁY lên (`Y` giảm dần theo thứ tự node). Đây
	// là hợp đồng của `domain/roadmap.ComputeLayout` — resolver chỉ chuyển tiếp,
	// test này bắt được nếu ai đó tình cờ "giúp" tính lại ở transport.
	//
	// `MEADOW` (không phải `PLAIN` như trước M7a): `enum Terrain` cũ khai sai
	// 6 giá trị, `terrainOf` rơi về `PLAIN` cho mọi giá trị ngoài `desert` ⇒
	// biên độ 0 và node nằm CHÍNH XÁC trên trục giữa. Sau M7a `amp(meadow)=40`
	// nên x dao động quanh 500.
	assert.InDelta(t, 500.0, stage.Topics[0].Point.X, 60,
		"node phải nằm quanh trục giữa ngang — lệch quá xa là layout không còn bám trục")
	require.Equal(t, domainroadmap.MapViewHeight-domainroadmap.MapMargin, stage.Topics[0].Point.Y,
		"node đầu nằm ở đáy trong lề an toàn (MapViewHeight - MapMargin)")
	require.Less(t, stage.Topics[1].Point.Y, stage.Topics[0].Point.Y, "node sau phải cao hơn node trước khi đi lên")
	require.False(t, stage.Topics[0].MapPinned)

	// Tầng 4: resource.
	require.Len(t, stage.Topics[0].Resources, 2)
	require.NotEmpty(t, stage.Topics[0].Resources[0].GUID)
	require.Equal(t, "VIDEO", stage.Topics[0].Resources[0].Kind)
}

// Test N+1 — số SQL của cây KHÔNG tăng theo số topic.
//
// Đây là gate của M4 (STACK-V2 §8: rủi ro "N+1 khi query cây roadmap 5 tầng" —
// mức CAO, giảm bằng dataloaden + ĐO số query ở gate).
//
// Tiêu chí: 1 topic và 51 topic phải tốn SỐ STATEMENT BẰNG NHAU. Nếu chênh lệch
// tuyến tính (×51) thì `dataloadgen` không bị dùng, và cây thật (51 topic / 263
// resource) sẽ tốn hơn 300 statement cho 1 request.
func Test_roadmap_tree_sql_statement_count_is_constant_as_topics_grow(t *testing.T) {
	// 1 khoá advisory, 2 schema: `pg_advisory_lock` không xếp hồng nên gọi
	// `newCountingHarness` 2 lần sẽ treo vô hạn.
	sess := platformtestdb.Acquire(t, context.Background())
	measure := func(slug string, n int) int64 {
		db, _ := sess.OpenSchema(t, context.Background())
		h, counter := countingHarness(t, db)
		seedPathWithTopics(t, h, slug, n)

		before := counter.Count()
		var resp struct {
			Path treePathResp `json:"path"`
		}
		h.client(t).MustPost(treeQuery, &resp, client.Var("slug", slug))
		after := counter.Count()

		// Chốt dữ liệu thật sự được đọc: 51 topic ⇒ 51 node trả về. Nếu cây rỗng
		// thì "số statement hằng" là vô nghĩa.
		got := 0
		for _, s := range resp.Path.Stages {
			got += len(s.Topics)
		}
		require.Equal(t, n, got, "cây phải trả đủ %d topic", n)
		return after - before
	}

	one := measure("p1", 1)
	fiftyOne := measure("p51", 51)

	t.Logf("SQL statement cho cây 5 tầng: 1 topic = %d, 51 topic = %d", one, fiftyOne)
	require.Equal(t, one, fiftyOne,
		"số SQL phải là HẰNG theo số topic (1 topic = %d, 51 topic = %d). "+
			"Chênh %d ⇒ dataloader không gom được, cây roadmap quay lại N+1.",
		one, fiftyOne, fiftyOne-one)
}

// Test N+1 riêng cho `paths { path { stages { … } } }` — loader phải gom key của
// MỌI path trong 1 request, không gom từng path một.
func Test_paths_list_sql_statement_count_is_constant_as_paths_grow(t *testing.T) {
	// 1 khoá advisory, 2 schema — lý do ở test trên.
	sess := platformtestdb.Acquire(t, context.Background())
	measure := func(prefix string, n int) int64 {
		db, _ := sess.OpenSchema(t, context.Background())
		h, counter := countingHarness(t, db)
		for i := 0; i < n; i++ {
			seedPathWithTopics(t, h, slugFor(prefix, i), 1)
		}
		before := counter.Count()
		var resp struct {
			Paths []struct {
				Path struct {
					Slug   string `json:"slug"`
					Stages []struct {
						Topics []struct {
							ID string `json:"id"`
						} `json:"topics"`
					} `json:"stages"`
				} `json:"path"`
			} `json:"paths"`
		}
		h.client(t).MustPost(`query { paths { path { slug stages { topics { id } } } } }`, &resp)
		require.Len(t, resp.Paths, n)
		// Chốt cây KHÔNG rỗng: 10 path mà `stages` rỗng thì "số SQL hằng" là vô
		// nghĩa vì không có gì để đọc.
		for _, p := range resp.Paths {
			require.Len(t, p.Path.Stages, 1, "path %s phải có stage", p.Path.Slug)
		}
		return counter.Count() - before
	}
	one := measure("q1", 1)
	ten := measure("q10", 10)
	t.Logf("SQL statement cho `paths { path { stages { topics } } }`: 1 path = %d, 10 path = %d", one, ten)
	require.Equal(t, one, ten,
		"số SQL của list path phải hằng (1 path = %d, 10 path = %d)", one, ten)
}

func slugFor(prefix string, i int) string {
	return fmt.Sprintf("%s-p%d", prefix, i)
}

// seedPathWithStages tạo 1 path + `n` stage, mỗi stage 1 topic (2 resource).
// MỌI stage gắn CHUNG 1 deck thật; stage đầu có 2 milestone, các stage sau 1.
//
// Vì sao cần biến thể này (F6): `seedPathWithTopics` LUÔN tạo đúng 1 stage,
// 0 milestone, `deck = nil`. Nghĩa là:
//   - nhánh `Stage.deck` (loader `deckRef`) không bao giờ chạy với key thật,
//   - `ListMilestonesByStageIDs` luôn chạy với mảng rỗng nên "hằng số" ấy là
//     hằng 0 — đo cái không có gì đo,
//   - số stage tăng lên KHÔNG được đo, nên ai đổi `stageTree` từ khoá theo
//     pathID sang khoá theo stageID vẫn giữ 2 test N+1 hiện có xanh.
//
// Deck gắn cho MỌI stage (không chỉ stage đầu) vì nếu chỉ 1 stage có deck thì
// một truy vấn deck-theo-từng-stage vẫn ra đúng 1 statement ⇒ test không bắt
// được. Hình dữ liệu này cũng sát thực tế: các stage của một path thường dùng
// chung bộ thẻ.
func seedPathWithStages(t *testing.T, h *harness, slug string, n int) {
	t.Helper()
	ctx := context.Background()
	_, err := h.container.Roadmap.CreatePath(ctx, roadmapapp.PathInput{
		Slug: slug, Title: "path " + slug, Language: "zh",
	})
	require.NoError(t, err)

	// Deck thật để `Stage.deck` có dữ liệu, và phải khớp `language` của path
	// (`resolveDeck` từ chối deck sai ngôn ngữ).
	deck, err := h.container.SRS.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err, "tạo deck để gắn stage")
	deckID := deck.ID

	for i := 0; i < n; i++ {
		st, err := h.container.Roadmap.CreateStage(ctx, slug, roadmapapp.StageInput{
			Slug:   fmt.Sprintf("%s-s%d", slug, i),
			Title:  fmt.Sprintf("giai doan %d", i),
			DeckID: &deckID,
		})
		require.NoError(t, err)
		require.NotZero(t, st.ID)

		// Milestone: stage đầu 2 cái để không lọt khỏi nhánh "nhiều hơn 1",
		// các stage sau 1 cái.
		mCount := 1
		if i == 0 {
			mCount = 2
		}
		for m := 0; m < mCount; m++ {
			_, err := h.container.Roadmap.CreateMilestone(ctx, st.ID,
				roadmapapp.MilestoneInput{Text: fmt.Sprintf("moc %d-%d", i, m)})
			require.NoError(t, err)
		}

		tp, err := h.container.Roadmap.CreateTopic(ctx, st.ID, roadmapapp.TopicInput{
			Title: fmt.Sprintf("chu de s%d", i),
		})
		require.NoError(t, err)
		for j := 0; j < 2; j++ {
			_, err := h.container.Roadmap.CreateResource(ctx, tp.ID, roadmapapp.ResourceInput{
				Title: "tai lieu", Kind: "video",
			})
			require.NoError(t, err)
		}
	}
}

// Test N+1 theo SỐ STAGE — chiều còn thiếu (F6).
//
// 2 test N+1 của M4 chỉ đo chiều "số topic tăng" và "số path tăng", trong khi
// seed luôn cố định 1 stage / 0 milestone / deck = nil. Ở shape đó:
//   - loader `deckRef` không bao giờ được gọi với key thật,
//   - `ListMilestonesByStageIDs` chạy với mảng rỗng (đo 1 lệnh trả 0 dòng),
//   - nếu `Stage.deck` bị đổi từ loader sang gọi `FindDecks` mỗi stage, hoặc
//     milestone bị đổi sang đọc từng stage, thì 2 test cũ VẪN XANH vì số stage
//     không đổi.
//
// Test này chèn cả 2 tầng còn thiếu vào dữ liệu thật rồi đo lại: 1 stage và
// 5 stage phải tốn SỐ STATEMENT BẰNG NHAU.
func Test_roadmap_tree_sql_statement_count_is_constant_as_stages_grow(t *testing.T) {
	// 1 khoá advisory, 2 schema — `pg_advisory_lock` không xếp hồng.
	sess := platformtestdb.Acquire(t, context.Background())
	measure := func(slug string, n int) int64 {
		db, _ := sess.OpenSchema(t, context.Background())
		h, counter := countingHarness(t, db)
		seedPathWithStages(t, h, slug, n)

		before := counter.Count()
		var resp struct {
			Path treePathResp `json:"path"`
		}
		h.client(t).MustPost(treeQuery, &resp, client.Var("slug", slug))
		after := counter.Count()

		// Chốt dữ liệu thật sự được đọc, kể cả các nhánh trước đây không có
		// dữ liệu: N stage, mỗi stage CÓ deck và CÓ milestone.
		require.Len(t, resp.Path.Stages, n, "cây phải trả đủ %d stage", n)
		totalTopics := 0
		for i, s := range resp.Path.Stages {
			totalTopics += len(s.Topics)
			wantMS := 1
			if i == 0 {
				wantMS = 2
			}
			require.NotNil(t, s.Deck,
				"mọi stage phải có deck — nếu nil thì loader deckRef không được đo")
			require.Len(t, s.Milestones, wantMS,
				"stage %d phải có %d milestone — nếu 0 thì nhánh ListMilestones chưa chạy", i, wantMS)
		}
		require.Equal(t, n, totalTopics, "mỗi stage 1 topic")
		return after - before
	}

	one := measure("s1", 1)
	five := measure("s5", 5)
	t.Logf("SQL statement cho cây 5 tầng (có deck + milestone): 1 stage = %d, 5 stage = %d", one, five)
	require.Equal(t, one, five,
		"số SQL phải hằng theo số STAGE (1 stage = %d, 5 stage = %d). "+
			"Chênh %d ⇒ tầng stage đang đọc theo từng stage, quay lại N+1.",
		one, five, five-one)
}
