module github.com/chaosbank/chaosbank/services/api-gateway

go 1.21

require (
	github.com/chaosbank/chaosbank v0.0.0
	github.com/gorilla/mux v1.8.0
)

require github.com/go-chi/chi/v5 v5.0.14 // indirect

replace github.com/chaosbank/chaosbank => ../../
