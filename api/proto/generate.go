// Package audiopb chứa code sinh từ `proto/audio/v1/audio.proto`.
//
// KHÔNG SỬA TAY file trong thư mục này — chạy `go generate ./proto/...` để tái
// lập. Lệnh đó cần 2 tool trong PATH:
//
//	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
//	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
//	go install github.com/bufbuild/buf/cmd/buf@latest
//
// Vì sao `buf` chứ không phải `protoc`: buf tự chứa compiler, không cần
// hệ thống có protoc/protoc-gen cài sẵn — chỉ cần 3 binary Go ở trên.
// `proto/buf.yaml` khai workspace (v2) để `buf lint` được.
package audiopb

//go:generate buf generate --template buf.gen.yaml
