#!/bin/bash
# Tạo database TEST `langapp_test` ngay khi Postgres khởi tạo volume.
#
# VÌ SAO 1 CONTAINER, 2 DATABASE (thay vì service `postgres-test` riêng):
#
#  1. `pg_trgm` và toàn bộ DDL của app đã được test trên Postgres 18 — thêm
#     1 container thứ hai có nghĩa là 1 image nữa phải kéo, 1 volume nữa, và
#     2 lần `initdb` (chậm, và lệch phiên bản patch là khó debug).
#  2. Test KHÔNG cần `max_connections` cao. Cả 2 database nằm chung instance
#     thì tổng connection vẫn nhỏ; tách container sẽ nhân lên số pool mà
#     `go test ./...` vốn đã kén (xem `open.go` — 2 conn/pool × 2 pool/package).
#  3. Database test là thứ SẼ BỊ XOÁ` (`make test-db-reset`, `docker compose
#     down -v`) — nó không đáng để tách service. Rủi ro lớn nhất ở đây là mất
#     dữ liệu APP; dữ liệu app nằm ở `langapp`, tách riêng hẳn là bảo vệ nó.
#
# VÌ SAO `IF NOT EXISTS`: script này chạy MỘT LẦN lúc image Postgres initdb
# volume rỗng. Nhưng nếu volume đã có sẵn (đây đúng tình trạng của máy dev),
# script KHÔNG chạy lại ⇒ `make db-up` vẫn phải tự tạo idempotent. Hai lớp,
# không lớp nào dựa vào lớp kia.
set -euo pipefail

TEST_DB="${TEST_DB:-langapp_test}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
	SELECT 'CREATE DATABASE "${TEST_DB}"'
	 WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = '${TEST_DB}')
	\gexec
EOSQL

echo "✔ đã đảm bảo database test '${TEST_DB}' tồn tại"
