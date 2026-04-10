module github.com/chaosbank/chaosbank/services/transaction-service

go 1.21

require (
	github.com/chaosbank/chaosbank v0.0.0
	github.com/lib/pq v1.10.9
)

require github.com/go-chi/chi/v5 v5.0.14

require github.com/google/uuid v1.6.0 // indirect

replace github.com/chaosbank/chaosbank => ../../
