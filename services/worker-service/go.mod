module github.com/chaosbank/chaosbank/services/worker-service

go 1.21

require (
	github.com/chaosbank/chaosbank v0.0.0
	github.com/segmentio/kafka-go v0.4.42
)

require (
	github.com/go-chi/chi/v5 v5.0.14 // indirect
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
)

replace github.com/chaosbank/chaosbank => ../../
