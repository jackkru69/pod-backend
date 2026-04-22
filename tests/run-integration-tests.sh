#!/bin/bash
# Integration test runner with Docker Compose (T132)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Pod Backend Integration Tests ==="
echo "Project root: $PROJECT_ROOT"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
COMPOSE_FILE="$SCRIPT_DIR/testdata/docker-compose.test.yaml"
TEST_TIMEOUT="5m"
TEST_PG_HOST_PORT="${TEST_PG_HOST_PORT:-15433}"
TEST_TON_CENTER_HOST_PORT="${TEST_TON_CENTER_HOST_PORT:-18082}"
TEST_BACKEND_HOST_PORT="${TEST_BACKEND_HOST_PORT:-13001}"

COMPOSE_CMD=()

detect_compose_cmd() {
    if docker compose version > /dev/null 2>&1; then
        COMPOSE_CMD=(docker compose)
        return 0
    fi

    if command -v docker-compose > /dev/null 2>&1; then
        COMPOSE_CMD=(docker-compose)
        return 0
    fi

    return 1
}

# Function to cleanup
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    cd "$PROJECT_ROOT"
    if [ ${#COMPOSE_CMD[@]} -gt 0 ]; then
        "${COMPOSE_CMD[@]}" -f "$COMPOSE_FILE" down -v
    fi
    echo -e "${GREEN}Cleanup complete${NC}"
}

# Trap cleanup on exit
trap cleanup EXIT

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: docker is not installed${NC}"
    exit 1
fi

if ! detect_compose_cmd; then
    echo -e "${RED}Error: neither 'docker compose' nor 'docker-compose' is available${NC}"
    exit 1
fi

echo -e "${GREEN}Using compose command:${NC} ${COMPOSE_CMD[*]}"

export TEST_PG_HOST_PORT
export TEST_TON_CENTER_HOST_PORT
export TEST_BACKEND_HOST_PORT

# Start test environment
echo -e "\n${YELLOW}Starting test environment...${NC}"
cd "$PROJECT_ROOT"
"${COMPOSE_CMD[@]}" -f "$COMPOSE_FILE" up -d postgres-test mock-toncenter

# Wait for services to be healthy
echo -e "${YELLOW}Waiting for services to be ready...${NC}"
sleep 5

# Check PostgreSQL
echo -n "PostgreSQL: "
if "${COMPOSE_CMD[@]}" -f "$COMPOSE_FILE" exec -T postgres-test pg_isready -U postgres > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗ Failed${NC}"
    exit 1
fi

# Check Mock TON Center
echo -n "Mock TON Center: "
if curl -f "http://localhost:${TEST_TON_CENTER_HOST_PORT}/health" > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗ Failed${NC}"
    exit 1
fi

# Prepare database schema
echo -e "\n${YELLOW}Preparing database schema...${NC}"
export TEST_PG_URL="postgresql://postgres:postgres@localhost:${TEST_PG_HOST_PORT}/pod_game_test?sslmode=disable"
export PG_URL="$TEST_PG_URL"
export TEST_TON_CENTER_URL="http://localhost:${TEST_TON_CENTER_HOST_PORT}"
export TON_CENTER_V2_URL="${TEST_TON_CENTER_URL}/api/v2"

schema_initialized="$(${COMPOSE_CMD[@]} -f "$COMPOSE_FILE" exec -T postgres-test psql -U postgres -d pod_game_test -Atqc "SELECT CASE WHEN to_regclass('public.users') IS NOT NULL THEN 'yes' ELSE 'no' END")"

if [ "$schema_initialized" = "yes" ]; then
    echo -e "${GREEN}Schema already initialized via docker-entrypoint-initdb.d${NC}"
elif command -v migrate &> /dev/null; then
    migrate -path "$PROJECT_ROOT/migrations" -database "$TEST_PG_URL" up
    echo -e "${GREEN}Migrations applied${NC}"
else
    echo -e "${RED}Error: database schema is not initialized and 'migrate' is unavailable${NC}"
    exit 1
fi

# Run integration tests
echo -e "\n${YELLOW}Running integration tests...${NC}"
cd "$PROJECT_ROOT"

# Run tests with coverage
TEST_RESULTS=0

echo -e "\n${YELLOW}[1/1] Running package integration suite against isolated services...${NC}"
if go test -v -count=1 -timeout "$TEST_TIMEOUT" ./tests/integration/...; then
    echo -e "${GREEN}✓ Integration tests passed${NC}"
else
    echo -e "${RED}✗ Integration tests failed${NC}"
    TEST_RESULTS=1
fi

# Print summary
echo -e "\n=== Test Summary ==="
if [ $TEST_RESULTS -eq 0 ]; then
    echo -e "${GREEN}All integration tests passed!${NC}"
else
    echo -e "${RED}Some integration tests failed${NC}"
fi

exit $TEST_RESULTS
