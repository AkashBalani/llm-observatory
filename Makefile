run:
	go run ./cmd/proxy

build:
	go build -o llm-observatory ./cmd/proxy

up:
	docker compose up --build

down:
	docker compose down

test:
	go test ./...

lint:
	golangci-lint run

# Send a test request through the proxy (requires ANTHROPIC_API_KEY set)
test-anthropic:
	curl -X POST http://localhost:8080/anthropic/v1/messages \
		-H "Content-Type: application/json" \
		-H "x-api-key: $(ANTHROPIC_API_KEY)" \
		-H "anthropic-version: 2023-06-01" \
		-d '{"model":"claude-haiku-4-5-20251001","max_tokens":64,"messages":[{"role":"user","content":"Say hello in one sentence."}]}'
