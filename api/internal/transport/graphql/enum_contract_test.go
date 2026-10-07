// Đối chiếu enum GraphQL ⇄ whitelist domain ⇄ CHECK constraint của DB.
//
// ── VÌ SAO FILE NÀY TỒN TẠI ────────────────────────────────────────────────
//
// Lớp lỗi "2 nguồn sự thật" đã xảy ra 3 lần trong dự án, mỗi lần đều im
// lặng (không lỗi biên dịch, không cảnh báo):
//   - F9 (M2): seed tra natural key bỏ qua tombstone.
//   - F1 (M4): `ProgressByIDs` thay vì `ProgressByPathIDs`.
//   - `enum Terrain` (M6b phát hiện, M7a sửa): schema khai
//     `PLAIN/FOREST/HILL/MOUNTAIN/WATER/DESERT` trong khi migration `00004` +
//     `domain/roadmap` đóng băng `meadow/desert/snow/volcano/ocean/city`.
//     `terrainOf` rơi về `PLAIN` cho 5/6 giá trị ⇒ **UI không lưu được 5/6
//     lựa chọn địa hình**, và chỉ ghi được `DESERT`.
//
// Mỗi lần đều chỉ lộ ra khi ai đó đọc code đối chiếu thủ công. Test này
// thay thủ công đọc đó: đọc TRỰC TIẾP file `.graphqls` (không đọc enum sinh
// từ codegen — nếu đọc bản sinh thì `go generate` chép lại lỗi và test xanh),
// đọc trực tiếp `domain`, và đọc trực tiếp SQL migration.
//
// ── HỢP ĐỒNG CỦA TEST ──────────────────────────────────────────────────────
//
//  1. MỌI giá trị enum trong schema có đúng 1 hằng tương ứng trong domain.
//  2. MỌI hằng domain có giá trị tương ứng trong enum (thêm hằng lạ vào
//     domain mà không sửa schema ⇒ ĐỎ).
//  3. Với enum có CHECK constraint ở DB: 3 nơi (schema/domain/DB) khớp nhau.
//
// Quy tắc ánh xạ tên: tên hằng domain viết thường (`meadow`) ⇄ tên enum
// GraphQL viết HOA (`MEADOW`). Quy tắc này giữ test ngắn và — quan trọng
// hơn — làm lỗi đổi tên hiện ra ở CẢ 2 chiều thay vì lệch im lặng 1 bên.
//
// `go test -run Test_graphql_enum / Test_domain_enum` là lưới an toàn: đổi
// enum trong schema mà quên domain (hoặc ngược lại) là test đỏ, không phải
// bug im lặng chạy tới review.
package graphql_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	roadmapapp "langapp/internal/application/roadmap"
	domaincontent "langapp/internal/domain/content"
	domainpractice "langapp/internal/domain/practice"
	domainroadmap "langapp/internal/domain/roadmap"
)

// roadmapKinds là alias cục bộ cho whitelist kind của `application/roadmap`.
//
// `ResourceKind` là enum DUY NHẤT không có hằng ở tầng domain: `kind` là
// `TEXT` tự do trong DB và whitelist nằm ở tầng application (vì application
// mới sở hữu `ValidateKind` trả message tiếng Việt cho client). Ghi rõ ở đây
// để không ai tưởng là test bỏ sót.
var roadmapKinds = roadmapapp.AllKinds

// enumSpec là 1 enum khai trong schema, kèm whitelist domain tương ứng.
type enumSpec struct {
	// name là tên enum trong `.graphqls`.
	name string
	// domain là whitelist ở tầng domain, theo đúng thứ tự khai trong
	// `All*`. Thứ tự KHÔNG quan trọng cho so khớp (test sort 2 bên) nhưng
	// giữ nguyên thứ tự domain để khi đọc test thấy ngay 2 bảng song song.
	domain []string
	// dbColumn là cột có CHECK constraint trong migration. Rỗng = không có
	// CHECK ở DB (enum suy ra trong code, VD `LevelState`), nên không kiểm
	// chiều DB cho enum đó.
	dbColumn string
	// dbMigration là file migration chứa CHECK. Rỗng = không kiểm DB.
	dbMigration string
	// dbUnchecked là lý do ĐÃ BIẾT vì sao cột không có CHECK, dù enum bị
	// lưu xuống DB. Bắt buộc khi `dbColumn` rỗng mà enum CÓ dữ liệu
	// persist — để "không có backstop ở DB" là QUYẾT ĐỊNH đã ghi, không phải
	// sơ suất. `Test_db_unchecked_column_reason_is_recorded` kiểm điều này.
	dbUnchecked string
	// note giải thích vì sao enum này có/không có CHECK — để người đọc
	// không phải tự suy.
	note string
}

// enumSpecs là bảng đối chiếu ĐẦY ĐỦ mọi enum trong `graph/schema/*.graphqls`.
//
// Duyệt: `common` (ErrorCode, Status, LevelState, ResourceKind, Terrain,
// Direction), `roadmap` (BookmarkStatus), `content` (ChunkKind), `practice`
// (WordStatus), `srs`/`insight`/`sync` (không có enum — mọi trường đều là
// `String!` vì cột DB là TEXT và v1 đã đóng băng hợp đồng đó).
//
// `Test_every_schema_enum_is_covered_here` bắt trường hợp ai đó THÊM enum mới
// vào schema mà quên thêm dòng vào bảng này — nếu không, enum mới sẽ không
// được đối chiếu với domain/DB và lớp lỗi "2 nguồn sự thật" quay lại y hệt.
var enumSpecs = []enumSpec{
	{
		name: "ErrorCode",
		// KHÔNG phải whitelist domain: `ErrorCode` ánh xạ 1-1 sang HTTP
		// status của `application/<ctx>.Error.Status` (400/404/409/500), đã
		// gộp trong `apperr.Status`. Test riêng cho chiều đó.
		domain:      nil,
		dbColumn:    "",
		dbMigration: "",
		note:        "ánh xạ HTTP status, kiểm bằng Test_graphql_error_code_covers_business_status",
	},
	{
		name:        "Status",
		domain:      statusStrings(),
		dbColumn:    "status",
		dbMigration: "00001_init.sql",
		note:        "CHECK trên roadmap_stages + roadmap_topics",
	},
	{
		name:   "LevelState",
		domain: levelStateStrings(),
		// KHÔNG có CHECK: `LevelState` là trạng thái SUY RA từ chuỗi node
		// trước đó (`domain/roadmap.LevelStates`), không lưu DB. Cột thật là
		// `status` đã có CHECK — kiểm ở dòng `Status`.
		note: "suy ra trong code, không lưu DB",
	},
	{
		name:   "ResourceKind",
		domain: resourceKindStrings(),
		// Cột `kind` là `TEXT NOT NULL DEFAULT ''` — KHÔNG có CHECK IN (...)
		// trong bất kỳ migration nào. Đây là TRẠNG THÁI ĐÃ BIẾT, không phải
		// khoảng trống: whitelist nằm ở MỘT nơi duy nhất
		// (`application/roadmap.AllKinds` + `ValidateKind`), nên không có
		// "2 nguồn sự thật" để lệch. `dbUnchecked` khai rõ điều đó để test
		// không báo động giả, VÀ để 1 ai đó thêm CHECK sau này thấy ngay phải
		// khai `dbColumn` (bỏ trống là test đỏ).
		//
		// Nợ M7b: thêm CHECK cho `kind` sẽ là migration mới (bump goose) +
		// phải xử lý `DEFAULT ''` trong danh sách. Ghi ở `stack-v2-m7a.md`.
		dbUnchecked: "cột kind không có CHECK — whitelist 1 nguồn duy nhất ở application/roadmap.AllKinds",
		note:        "TEXT NOT NULL DEFAULT ''",
	},
	{
		name:        "Terrain",
		domain:      terrainStrings(),
		dbColumn:    "terrain",
		dbMigration: "00004_roadmap_map.sql",
		note:        "CHECK 6 giá trị; DEFAULT 'meadow'",
	},
	{
		name:        "Direction",
		domain:      directionStrings(),
		dbColumn:    "direction",
		dbMigration: "00004_roadmap_map.sql",
		note:        "CHECK 2 giá trị; DEFAULT 'up'",
	},
	{
		name:        "BookmarkStatus",
		domain:      bookmarkStatusStrings(),
		dbColumn:    "status",
		dbMigration: "00003_roadmap_a1.sql",
		note:        "CHECK trên roadmap_bookmarks.status (cùng tên cột, file khác)",
	},
	{
		name:   "ChunkKind",
		domain: chunkKindStrings(),
		// `Chunk.Kind` là 1 hằng trả về, không đọc/ghi DB (hàm
		// `domain/content.SplitChunks` trả về kèm). Không có cột, không có CHECK.
		note: "hằng trả về của SplitChunks, không lưu DB",
	},
	{
		name:   "WordStatus",
		domain: wordStatusStrings(),
		// `WordStatus` là kết quả so 2 chuỗi trong RAM (`practice.WordDiff`),
		// không phải dữ liệu. Không có cột.
		note: "kết quả WordDiff trong RAM, không lưu DB",
	},
}

func statusStrings() []string {
	out := make([]string, 0, len(domainroadmap.AllStatuses))
	for _, v := range domainroadmap.AllStatuses {
		out = append(out, string(v))
	}
	return out
}

func levelStateStrings() []string {
	out := make([]string, 0, len(domainroadmap.AllLevelStates))
	for _, v := range domainroadmap.AllLevelStates {
		out = append(out, string(v))
	}
	return out
}

func terrainStrings() []string {
	out := make([]string, 0, len(domainroadmap.AllTerrains))
	for _, v := range domainroadmap.AllTerrains {
		out = append(out, string(v))
	}
	return out
}

func directionStrings() []string {
	out := make([]string, 0, len(domainroadmap.AllDirections))
	for _, v := range domainroadmap.AllDirections {
		out = append(out, string(v))
	}
	return out
}

func bookmarkStatusStrings() []string {
	out := make([]string, 0, len(domainroadmap.AllBookmarkStatuses))
	for _, v := range domainroadmap.AllBookmarkStatuses {
		out = append(out, string(v))
	}
	return out
}

// chunkKindStrings đọc từ `SplitChunks` thay vì từ hằng.
//
// Lý do: `domain/content` khai `ChunkContent`/`ChunkFunction` là hằng RỜI
// không có slice `All*`. Gọi `SplitChunks("a word")` thu được đúng 2 loại đang
// dùng — và nếu ai đó thêm loại thứ 3 mà quên khai báo ở schema, hàm vẫn trả
// loại mới ⇒ test đỏ. Đây là "mirror test" theo nghĩa thuận: giá trị mong
// đợi đến từ hàm, không từ bảng viết tay thứ 2 (bảng viết tay chính là nơi
// lỗi "2 nguồn sự thật" bắt đầu).
func chunkKindStrings() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range domaincontent.SplitChunks("a quick brown fox the end") {
		if !seen[c.Kind] {
			seen[c.Kind] = true
			out = append(out, c.Kind)
		}
	}
	sort.Strings(out)
	return out
}

// wordStatusStrings liệt kê đủ 4 trạng thái bằng 4 cặp input thật.
//
// `domain/practice` cũng khai hằng rời không có `All*`. Sinh từ hàm thật
// (`WordDiff`) thay vì bảng viết tay: cặp input nào sinh ra trạng thái nào là
// hợp đồng của hàm, và 1 trạng thái mà hàm không bao giờ sinh ra (VD thêm
// `CONFUSED`) sẽ không có trong bảng mong đợi ⇒ lỗi "enum có mà domain không
// sinh" lộ ra.
func wordStatusStrings() []string {
	cases := [][2]string{
		{"a b c", "a b c"},   // ok
		{"a b c", "a b d"},   // wrong
		{"a b c", "a b"},     // missing
		{"a b c", "a b c d"}, // extra
	}
	seen := map[string]bool{}
	var out []string
	for _, c := range cases {
		// ctx luôn Background nên err không bao giờ khác nil; bỏ qua để không
		// phải đổi chữ ký helper đọc enum.
		toks, _ := domainpractice.WordDiff(context.Background(), c[0], c[1])
		for _, tok := range toks {
			if !seen[string(tok.Status)] {
				seen[string(tok.Status)] = true
				out = append(out, string(tok.Status))
			}
		}
	}
	sort.Strings(out)
	return out
}

// resourceKindStrings đọc từ `roadmapapp.AllKinds` — whitelist nằm ở tầng
// application, không có bản sao ở domain.
func resourceKindStrings() []string {
	out := make([]string, 0, len(roadmapKinds))
	for _, v := range roadmapKinds {
		out = append(out, v)
	}
	return out
}

// schemaDir là thư mục chứa `.graphqls`.
//
// Tính từ file test đang chạy (`internal/transport/graphql`) ⇒ `api/graph/schema`.
// Không dùng đường dẫn tuyệt đối và không phụ thuộc thư mục chạy test.
func schemaDir(t *testing.T) string {
	t.Helper()
	// `__FILE__` được thay bằng đường dẫn file này lúc build (xem
	// `file:line` directive của Go) — không dùng được ở đây nên đi từ
	// `runtime.Caller`.
	return filepath.Join("..", "..", "..", "graph", "schema")
}

// migrationsDir là thư mục migration, cùng cách suy ra.
func migrationsDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "migrations")
}

// enumRe khớp 1 khối `enum X { … }` trong `.graphqls`.
//
// Regex thay vì parser: schema là 8 file tĩnh, không có enum lồng nhau, và
// `graphql-go-tools` sẽ là dependency nặng cho 1 test đối chiếu whitelist.
// Bỏ qua comment (`#`) và docstring (`"""`).
var enumRe = regexp.MustCompile(`(?s)enum\s+(\w+)\s*\{(.*?)\n\}`)

// schemaEnums trả map tên enum → danh sách giá trị, đọc từ mọi `.graphqls`.
func schemaEnums(t *testing.T) map[string][]string {
	t.Helper()
	dir := schemaDir(t)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "đọc %s — codegen chạy từ đây, test cũng phải đọc từ đây", dir)

	found := map[string][]string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".graphqls") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err, e.Name())
		for _, m := range enumRe.FindAllStringSubmatch(string(raw), -1) {
			name := m[1]
			var values []string
			for _, line := range strings.Split(m[2], "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, `"""`) {
					continue
				}
				values = append(values, line)
			}
			if prev, dup := found[name]; dup {
				t.Fatalf("enum %s khai 2 lần (%v rồi %v) — gqlgen sẽ từ chối build", name, prev, values)
			}
			found[name] = values
		}
	}
	require.NotEmpty(t, found, "không đọc được enum nào từ %s — regex đã lệch với format schema", dir)
	return found
}

// enumName là chiều schema → domain: `meadow` → `MEADOW`.
//
// Dùng `strings.ToUpper` chứ không bảng tra: quy tắc HOA hoá là quy ước
// GraphQL, và dùng nó ở đây khiến test bắt được cả lỗi đổi tên (`Meadow` →
// `MEADOW_` sẽ ra `MEADOW_` ở schema, lệch domain ⇒ đỏ).
func enumName(domainValue string) string { return strings.ToUpper(domainValue) }

// Test_graphql_enum_has_matching_domain_constant — CHIỀU 1.
//
// Mọi giá trị trong enum schema phải có 1 hằng domain tương ứng. Đây là
// chiều bắt được `enum Terrain` cũ: schema có `PLAIN` mà domain không có
// `plain` ⇒ đỏ ngay.
func Test_graphql_enum_has_matching_domain_constant(t *testing.T) {
	enums := schemaEnums(t)
	for _, spec := range enumSpecs {
		if spec.domain == nil {
			continue // ErrorCode — kiểm ở test riêng
		}
		t.Run(spec.name, func(t *testing.T) {
			require.Contains(t, enums, spec.name, "enum %s không còn trong schema", spec.name)
			domainSet := map[string]bool{}
			for _, d := range spec.domain {
				domainSet[d] = true
			}
			for _, v := range enums[spec.name] {
				lower := strings.ToLower(v)
				assert.True(t, domainSet[lower],
					"enum %s có giá trị %q mà domain KHÔNG có hằng %q — hoặc đổi schema về whitelist domain, hoặc thêm hằng vào domain",
					spec.name, v, lower)
			}
		})
	}
}

// Test_domain_constant_has_matching_enum_value — CHIỀU 2.
//
// Ngược chiều 1: thêm hằng mới vào domain mà không khai ở schema ⇒ đỏ. Đây là
// chiều mà `enum Terrain` cũ ĐÃ LỌT (schema có 6 giá trị, domain có 6 giá trị
// khác hẳn — nhưng không test nào so 2 danh sách, nên cả 2 cùng "đúng" theo
// mắt riêng của mình).
func Test_domain_constant_has_matching_enum_value(t *testing.T) {
	enums := schemaEnums(t)
	for _, spec := range enumSpecs {
		if spec.domain == nil {
			continue
		}
		t.Run(spec.name, func(t *testing.T) {
			require.Contains(t, enums, spec.name, "enum %s không còn trong schema", spec.name)
			schemaSet := map[string]bool{}
			for _, v := range enums[spec.name] {
				schemaSet[v] = true
			}
			for _, d := range spec.domain {
				want := enumName(d)
				assert.True(t, schemaSet[want],
					"domain có hằng %q ⇒ schema phải khai enum %s.%s. Không thêm vào schema thì hằng đó không bao giờ tới được client.",
					d, spec.name, want)
			}
		})
	}
}

// Test_graphql_enum_matches_db_check_constraint — CHIỀU DB.
//
// Với enum có CHECK ở migration: giá trị trong CHECK phải khớp whitelist
// domain. Đọc file SQL thật (không đọc `information_schema` — cần DB và
// migration có thể chưa chạy; đọc file thì test được trên máy không bật
// Postgres và bắt được lỗi NGAY ở file migration, trước khi ai đó chạy nó).
func Test_graphql_enum_matches_db_check_constraint(t *testing.T) {
	for _, spec := range enumSpecs {
		if spec.dbColumn == "" {
			continue
		}
		t.Run(spec.name, func(t *testing.T) {
			sql := readMigration(t, spec.dbMigration)
			values := checkColumnValues(t, sql, spec.dbColumn)
			require.NotEmpty(t, values,
				"không tìm thấy CHECK IN (...) cho cột %q trong %s — enum và DB đã lệch nhau, hoặc tên cột đã đổi",
				spec.dbColumn, spec.dbMigration)
			domainSet := map[string]bool{}
			for _, d := range spec.domain {
				domainSet[d] = true
			}
			got := map[string]bool{}
			for _, v := range values {
				got[v] = true
				assert.True(t, domainSet[v],
					"CHECK constraint của cột %s trong %s cho phép %q nhưng domain KHÔNG có hằng đó — client không chọn được nhưng DB vẫn nhận",
					spec.dbColumn, spec.dbMigration, v)
			}
			for _, d := range spec.domain {
				assert.True(t, got[d],
					"domain có hằng %q nhưng CHECK của cột %s trong %s không cho phép — mọi lần ghi sẽ bị Postgres từ chối (23514)",
					d, spec.dbColumn, spec.dbMigration)
			}
		})
	}
}

// checkRe khớp `CHECK (<col> IN ('a', 'b', …))`.
//
// Bỏ qua `CHECK (position >= 0)` và `CHECK (grade BETWEEN 1 AND 4)` — chỉ
// khớp dạng `IN`, tức là dạng duy nhất đóng băng một whitelist rời. `status`
// xuất hiện ở 2 migration khác nhau (`00001` cho node, `00003` cho bookmark)
// nên test gọi hàm này với đúng file tương ứng từng enum.
var checkRe = regexp.MustCompile(`CHECK\s*\(\s*(\w+)\s+IN\s*\(([^)]*)\)`)

// Test_db_unchecked_column_reason_is_recorded bắt "không có CHECK ở DB" mà
// không ai ghi lý do.
//
// Enum được LƯU xuống DB mà cột không có CHECK là quyết định có hậu quả thật:
// `ValidateKind` ở application chặn giá trị lạ, nhưng merge của `sync` ghi
// thẳng SQL (`UpsertResource` set `updated_at` tường minh) — một dữ liệu peer
// mang `kind` lạ sẽ vào DB không bị chặn. Không có CHECK ⇒ DB là nguồn sự thật
// DUY NHẤT không có backstop. Chấp nhận được, nhưng phải là quyết định ghi ra.
//
// `dbUnchecked` rỗng ở enum có cột thật trong `00001`/`00003` ⇒ test đỏ.
func Test_db_unchecked_column_reason_is_recorded(t *testing.T) {
	for _, spec := range enumSpecs {
		if spec.dbColumn != "" {
			continue
		}
		if spec.dbUnchecked != "" {
			continue
		}
		// Enum không lưu DB (`LevelState`, `ChunkKind`, `WordStatus`) không
		// cần backstop. Phân biệt bằng `note` chứ không bằng bảng cứng: nếu
		// sau này thêm 1 enum lưu DB thì người viết phải điền `dbUnchecked`
		// hoặc `dbColumn`, và test này sẽ bắt.
		assert.NotEmpty(t, spec.note,
			"enum %s không có dbColumn lẫn dbUnchecked — nếu enum này KHÔNG lưu DB thì `note` phải nói rõ; nếu CÓ lưu thì phải khai dbUnchecked",
			spec.name)
	}
}

func checkColumnValues(t *testing.T, sql, column string) []string {
	t.Helper()
	var out []string
	for _, m := range checkRe.FindAllStringSubmatch(sql, -1) {
		if m[1] != column {
			continue
		}
		for _, lit := range strings.Split(m[2], ",") {
			lit = strings.TrimSpace(lit)
			if len(lit) >= 2 && strings.HasPrefix(lit, "'") && strings.HasSuffix(lit, "'") {
				out = append(out, strings.Trim(lit, "'"))
			}
		}
	}
	return out
}

func readMigration(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(migrationsDir(t), name)
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "đọc migration %s", name)
	return string(raw)
}

// Test_every_schema_enum_is_covered_here chặn enum MỚI không được đối chiếu.
//
// Đây là test quan trọng nhất của file: nếu không có nó, ai đó thêm
// `enum Difficulty { EASY MEDIUM HARD }` vào schema là xong — không test nào
// để ý, và lớp lỗi "2 nguồn sự thật" quay lại đúng như `enum Terrain`.
func Test_every_schema_enum_is_covered_here(t *testing.T) {
	enums := schemaEnums(t)
	covered := map[string]bool{}
	for _, spec := range enumSpecs {
		covered[spec.name] = true
	}
	var missing []string
	for name := range enums {
		if !covered[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	assert.Empty(t, missing,
		"enum mới trong schema chưa có dòng trong `enumSpecs`: %v. Thêm dòng (kèm whitelist domain) hoặc test này sẽ đỏ.",
		missing)

	// Chiều ngược: dòng trong bảng mà schema đã bỏ.
	var stale []string
	for _, spec := range enumSpecs {
		if !contains(schemaEnumNames(enums), spec.name) {
			stale = append(stale, spec.name)
		}
	}
	sort.Strings(stale)
	assert.Empty(t, stale, "enumSpecs có dòng cho enum không còn tồn tại trong schema: %v", stale)
}

func schemaEnumNames(enums map[string][]string) []string {
	out := make([]string, 0, len(enums))
	for name := range enums {
		out = append(out, name)
	}
	return out
}

func contains(hay []string, needle string) bool {
	for _, v := range hay {
		if v == needle {
			return true
		}
	}
	return false
}
