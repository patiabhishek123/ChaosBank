module github.com/chaosbank/chaosbank/services/worker-service

go 1.21

require (
	github.com/chaosbank/chaosbank v0.0.0
	github.com/segmentio/kafka-go v0.4.42
	github.com/lib/pq v1.10.9
)

replace github.com/chaosbank/chaosbank => ../../