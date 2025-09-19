# Building Go HTMX app

curl -X POST "http://localhost:5000/subjects/ai"   -H "Content-Type: text/plain"   --data-binary "hello world"
curl -X POST "http://localhost:7777/users/1/subjects/ai" \
  -H "Content-Type: text/plain" \
  --data-binary 'neural networks are everywhere!


[{"UserID":"1","Subjects":[{"Name":"ai","Thoughts":["neural networks are everywhere!"]}]}]

### Minimal Target Layout
.
├── bin/                        # optional, where you ‘go build -o’ to
├── cmd/
│   └── api/
│       ├── main.go             # flags, logger, server timeouts, DI via application{}
│       ├── routes.go           # httprouter routes
│       └── healthcheck.go      # /v1/healthcheck
├── internal/                   # placeholder (used heavily in later chapters)
├── go.mod
├── go.sum
├── design_docs.md
├── mvp_schema.dbml
├── README.md
# (your existing libs/tests can stay; we’ll wire them gradually)
├── file_system_store.go
├── file_system_store_test.go
├── in_memory_player_store.go
├── server.go
├── server_integration_test.go
├── server_test.go
├── tape.go
└── tape_test.go


