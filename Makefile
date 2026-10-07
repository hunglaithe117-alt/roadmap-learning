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
# schema giữ extension `pg_trgm`. Trước đây schema đó là `public` — CÙNG schema
# chứa bảng thật của app — nên `SELECT count(*) FROM dict` sau khi test đã
# `DROP TABLE dict` sẽ RƠI XUỐNG `public` và đọc dữ liệu thật của user, thay
# vì báo "relation does not exist" ⇒ test xanh vì đọc nhầm bảng production.
# Tệ hơn: test có thể GHI vào bảng thật.
#
# Nay `search_path` test dùng schema `testext` (xem `testdb.ExtSchema`) chứ
# KHÔNG dùng `public` — nhưng database riêng vẫn là lớp cách ly CHÍNH, vì nó
# bảo vệ dữ liệu ngay cả khi `search_path` bị cấu hình sai.
#
# `testdb.requireIsolatedDB` fail nếu DSN trỏ vào database có dữ liệu app (so
# cả tên lẫn bằng chứng cấu trúc), nên quên đổi DSN sẽ ra lỗi to+ nét thay
# vì kết quả sai. Database này do `docker/postgres-init/` tạo lúc Postgres
# init; `db-up` dưới đây tạo lại idempotent cho volume đã có sẵn.
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
# ⚠️ CẢNH BÁO TRƯỚC, VÌ `db-up` KHÔNG ĐỦ.
#
# `test` chạy vào database `langapp_test`, KHÔNG phải database app — nhưng có 1
# điều kiện ngầm mà `db-up` không lo được: app phải đã BOOT ÍT NHẤT 1 LẦN để
# extension `pg_trgm` tồn tại trong `langapp`. `pg_trgm` là extension cấp
# database, `00002_fts.sql` cài nó bằng `CREATE EXTENSION IF NOT EXISTS` vào
# schema đầu tiên của `search_path` app (tức `public`).
#
# Nếu `langapp` CHƯA TỪNG BOOT (đúng trạng thái sau `docker compose down -v`, tức
# bước 1 của "Reset" trong DEPLOY.md) thì `pg_trgm` chưa có ở đâu cả. Khi đó
# lớp tự vệ trong `testdb` SẼ CHẶN (nó không ghi vào database app), nhưng bài
# test viện cảnh báo này vẫn ĐỎ — đúng mục đích, vì "app chưa boot" là trạng
# thái chưa sẵn sàng để test.
#
# Nếu bạn vừa `down -v`, hãy boot app TRƯỚC rồi hãy `make test`:
#
#	docker compose --profile v2 down -v
#	docker compose --profile v2 up -d     # chờ healthy
#	make test
#
# Xem `DEPLOY.md` § "Chạy test".
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

# ── Codegen ────────────────────────────────────────────────────────────────
#
# Codegen cần 4 binary ngoài PATH: buf, protoc-gen-go, protoc-gen-go-grpc và
# gqlgen. `check-codegen-tools` chặn TRƯỚC khi chạy codegen để thiếu binary ra
# thông báo kèm lệnh cài, thay vì lỗi "executable file not found" khó hiểu.
#
# Chú ý comment trong recipe: KHÔNG đặt dòng `#` bắt đầu bằng tab BÊN TRONG
# recipe (make nuốt dòng lệnh kế tiếp, xem ghi chú ở mục Hạ tầng). Vì vậy mọi
# chú thích ở đây đặt NGOÀI recipe.

.PHONY: check-codegen-tools
check-codegen-tools: ## Fail nếu thiếu tool codegen (buf, protoc-gen-go, protoc-gen-go-grpc, gqlgen)
	@missing=""; for t in buf protoc-gen-go protoc-gen-go-grpc gqlgen; do \
		command -v $$t >/dev/null 2>&1 || missing="$$missing $$t"; done; \
	if [ -n "$$missing" ]; then \
		echo "✗ thiếu tool codegen:$$missing"; \
		echo "  cài: go install github.com/bufbuild/buf/cmd/buf@latest"; \
		echo "       go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"; \
		echo "       go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"; \
		echo "       go install github.com/99designs/gqlgen@latest"; \
		echo "  binary nằm ở \$$(go env GOPATH)/bin — thêm vào PATH nếu chưa có"; \
		exit 1; fi

.PHONY: generate
generate: check-codegen-tools ## Chạy codegen (gqlgen + buf). Cần 4 tool trong PATH.
	cd api/internal/transport/graphql && go generate ./...
	cd api/proto && go generate ./...

.PHONY: generate-check
generate-check: check-codegen-tools ## So hash: fail nếu codegen sinh ra khác nguồn (hoặc go.sum đổi)
	@cd api && find internal/transport/graphql/generated internal/transport/graphql/model proto -type f -name '*.go' \
		-exec sha256sum {} + | sort > /tmp/langapp-gen-before.txt
	@cd api && cp go.sum /tmp/langapp-go.sum.bak
	@cd api/internal/transport/graphql && go generate ./...
	@cd api/proto && go generate ./...
	@cd api && find internal/transport/graphql/generated internal/transport/graphql/model proto -type f -name '*.go' \
		-exec sha256sum {} + | sort > /tmp/langapp-gen-after.txt
	@if ! diff -q /tmp/langapp-gen-before.txt /tmp/langapp-gen-after.txt >/dev/null; then \
		echo "✗ generated code lệch — chạy 'make generate' rồi commit"; \
		diff /tmp/langapp-gen-before.txt /tmp/langapp-gen-after.txt || true; \
		exit 1; fi
	@cd api && if ! diff -q go.sum /tmp/langapp-go.sum.bak >/dev/null; then \
		echo "✗ go generate sinh khác biệt trong go.sum"; \
		diff go.sum /tmp/langapp-go.sum.bak || true; exit 1; fi; \
	echo "✔ generated code khớp nguồn; go.sum không đổi"

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
