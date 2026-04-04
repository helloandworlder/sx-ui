# sx-ui E2E Test Makefile
# Run from this directory or monorepo root with:
#   make -C sx-ui/test/e2e test

COMPOSE = docker compose -f docker-compose.yml
ROOT = ../../..

.PHONY: test build clean logs

# Build + run full E2E test suite
test: build
	cd $(ROOT) && $(COMPOSE) -f sx-ui/test/e2e/docker-compose.yml up --abort-on-container-exit --exit-code-from test-runner
	@echo "✓ E2E tests completed"

# Build images only
build:
	cd $(ROOT) && docker build -f sx-ui/test/e2e/Dockerfile.sx-ui -t sx-ui-test .

# Clean up
clean:
	cd $(ROOT) && $(COMPOSE) -f sx-ui/test/e2e/docker-compose.yml down -v --remove-orphans

# Show logs
logs:
	cd $(ROOT) && $(COMPOSE) -f sx-ui/test/e2e/docker-compose.yml logs -f
