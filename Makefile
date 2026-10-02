# Makefile — entrypoint DUY NHẤT để chạy test.
#
# Vì sao cần file này: gate M6 chạy `go test ./...` không set DSN và báo
# "525 PASS / 219 SKIP / 0 FAIL". Con số "0 FAIL" đó VÔ NGHĨA khi 1/3 suite
# im lặng bỏ qua — và đúng loại đó đã giấu bug mất dữ liệu ở chiều ghi (F1)
# suốt 2 phase. `go test` không có khả năng "fail vì chưa set biến" nên phải
# có 1 chỗ duy nhất gom DSN + typecheck lại, thay vì mỗi người tự nhớ.
#
# KHÔNG thêm GitHub Actions: user đã quyết không dùng git, workflow sẽ vô dụng.
# Cách kiểm duy nhất là đúng cái này: `make check`.

SHELL := /bin/bash
.DEFAULT_GOAL := help

# ── Cấu hình ────────────────────────────────────────────────────────────────

# Postgres test chạy ở cổng HOST 5433 (đổi vì máy dev đã có Postgres ở 5432) —
# khớp `docker-compose.yml`. Ghi đè được: `POSTGRES_PORT=5555 make test`.
POSTGRES_PORT ?= 5433
POSTGRES_PASSWORD ?= langapp

# Database TEST RIÊNG, không phải database app.
#
# VÌ SAO PHẢI RIÊNG (đây là bài học thật, không phải phòng xa): test dựng
# schema tạm rồi `search_path` trỏ vào đó, nhưng `search_path` buộc phải có
# schema giữ extension `pg_trgm` — mà extension đó thường nằm ở `public`,
# CÙNG schema chứa bảng thật của app. Khi đó `SELECT count(*) FROM dict` sau
# khi test đã `DROP TABLE dict` sẽ RƠI XUỐNG `public` và đọc dữ liệu thật của
# user, thay vì báo "relation does not exist" ⇒ test xanh vì đọc nhầm bảng
# production. Test còn có thể GHI vào bảng thật.
#
# `testdb.requireIsolatedDB` fail nếu DSN trỏ vào database có dữ liệu app, nên
# quên đổi DSN sẽ ra lỗi to+ nét thay vì kết quả sai.
TEST_DB ?= langapp_test

# DSN có sẵn thì giữ nguyên (để CI/agent truyền DSN riêng được), không thì dựng
# từ các biến trên. `?sslmode=disable` là local.
LANGAPP_TEST_POSTGRES_DSN ?= postgres://langapp:$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_PORT)/$(TEST_DB)?sslmode=disable

# Thời gian timeout cho 1 lần chạy test. Test DB dựng schema + migration thật
# nên lâu hơn `go test` mặc định (10m) khi chạy tuần tự trên máy chậm.
GO_TEST_TIMEOUT ?= 15m

# ── Test ────────────────────────────────────────────────────────────────────

# `db-up` là prerequisite CỐ Ý: `docker compose down -v` xoá cả volume Postgres
# ⇒ database `langapp_test` biến mất. Nếu `test` không tự dựng lại thì toàn bộ
# suite DB fail bằng `database "langapp_test" does not exist` — và người đọc log
# sẽ tưởng code hỏng, trong khi thực ra chỉ là thiếu 1 bước dựng hạ tầng.
.PHONY: test
test: db-up ## Chạy test backend với DSN thật (tự trỏ database test riêng)
	@echo "▶ go test ./...  (DSN: $${LANGAPP_TEST_POSTGRES_DSN##*@})"
	cd api && LANGAPP_TEST_POSTGRES_DSN="$(LANGAPP_TEST_POSTGRES_DSN)" \
		go test -count=1 -timeout $(GO_TEST_TIMEOUT) ./...

.PHONY: test-web
test-web: ## Test frontend + typecheck (vitest KHÔNG typecheck — phải chạy cả 2)
	@echo "▶ pnpm vitest run"
	cd web && pnpm vitest run
	@echo "▶ pnpm typecheck (bắt buộc: vitest không typecheck)"
	cd web && pnpm typecheck

.PHONY: check
check: test test-web vet build ## Kiểm tra đầy đủ: test + typecheck + vet + build
	@echo "✔ check xong"

.PHONY: vet
vet:
	cd api && go vet ./...

.PHONY: build
build:
	cd api && go build ./...

.PHONY: fmt-check
fmt-check: ## Fail nếu có file Go chưa gofmt
	@out=$$(cd api && gofmt -l . ); \
	if [ -n "$$out" ]; then echo "✗ chưa gofmt:"; echo "$$out"; exit 1; fi; \
	echo "✔ gofmt sạch"

.PHONY: generate-check
generate-check: ## Fail nếu `go generate` sinh ra khác biệt (idempotent)
	@cd api && cp go.sum /tmp/langapp-go.sum.bak && \
	 go generate ./... >/dev/null 2>&1; \
	if ! diff -q go.sum /tmp/langapp-go.sum.bak >/dev/null; then \
		echo "✗ go generate sinh khác biệt trong go.sum"; exit 1; fi; \
	echo "✔ go generate idempotent"

# ── Hạ tầng ────────────────────────────────────────────────────────────────

# `pg_isready` in ra "<socket> - accepting connections" nên phải so khớp bằng
# `grep`, KHÔNG so bằng `=` — so sánh chuỗi nguyên văn sẽ treo vĩnh viễn.
# Ghi chú đặt ở ĐÂY (ngoài recipe) vì dòng `#` bắt đầu bằng tab BÊN TRONG recipe
# làm make nuốt luôn dòng lệnh kế tiếp — triệu chứng là lệnh biến mất im lặng
# mà không báo lỗi.
.PHONY: db-up
db-up: ## Dựng Postgres + database test (idempotent)
	docker compose up -d postgres
	@echo "▶ chờ Postgres healthy…"
	@until docker compose exec -T postgres pg_isready -U langapp -d langapp 2>/dev/null | grep -q "accepting connections"; do sleep 1; done
	@docker compose exec -T postgres psql -U langapp -d postgres \
		-c "SELECT 'CREATE DATABASE \"$(TEST_DB)\"' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname='$(TEST_DB)')" \
		| grep -q 'CREATE DATABASE' \
		&& docker compose exec -T postgres psql -U langapp -d postgres -c 'CREATE DATABASE "$(TEST_DB)"' \
		|| true
	@docker compose exec -T postgres psql -U langapp -d postgres -tc "SELECT 1 FROM pg_database WHERE datname='$(TEST_DB)'" | grep -q 1 \
		|| { echo "✗ không tạo được database $(TEST_DB)"; exit 1; }
	@echo "✔ Postgres + database $(TEST_DB) sẵn sàng"

.PHONY: test-db-reset
test-db-reset: ## Xoá sạch database test (dữ liệu rác từ run trước)
	docker compose exec -T postgres psql -U langapp -d postgres \
		-c "DROP DATABASE IF EXISTS $(TEST_DB) WITH (FORCE)"
	@$(MAKE) db-up

.PHONY: help
help: ## Liệt kê target
	@grep -hE '^[a-z-]+:.*?##' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
