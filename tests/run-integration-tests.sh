#!/bin/bash
# Integration test runner with Docker Compose (T132)

set -e

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

# Function to cleanup
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    cd "$PROJECT_ROOT"
    docker-compose -f "$COMPOSE_FILE" down -v
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

if ! command -v docker-compose &> /dev/null; then
    echo -e "${RED}Error: docker-compose is not installed${NC}"
    exit 1
fi

# Start test environment
echo -e "\n${YELLOW}Starting test environment...${NC}"
cd "$PROJECT_ROOT"
docker-compose -f "$COMPOSE_FILE" up -d postgres-test mock-toncenter

# Wait for services to be healthy
echo -e "${YELLOW}Waiting for services to be ready...${NC}"
sleep 5

# Check PostgreSQL
echo -n "PostgreSQL: "
if docker-compose -f "$COMPOSE_FILE" exec -T postgres-test pg_isready -U postgres > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗ Failed${NC}"
    exit 1
fi

# Check Mock TON Center
echo -n "Mock TON Center: "
if curl -f http://localhost:8082/health > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗ Failed${NC}"
    exit 1
fi

# Run database migrations
echo -e "\n${YELLOW}Running database migrations...${NC}"
export TEST_PG_URL="postgresql://postgres:postgres@localhost:5433/pod_game_test?sslmode=disable"

if command -v migrate &> /dev/null; then
    migrate -path "$PROJECT_ROOT/migrations" -database "$TEST_PG_URL" up
    echo -e "${GREEN}Migrations applied${NC}"
else
    echo -e "${YELLOW}Warning: migrate not found, assuming database is already migrated${NC}"
fi

# Run integration tests
echo -e "\n${YELLOW}Running integration tests...${NC}"
cd "$PROJECT_ROOT"

# Set environment variables for tests
export TEST_PG_URL="postgresql://postgres:postgres@localhost:5433/pod_game_test?sslmode=disable"
export TEST_TON_CENTER_URL="http://localhost:8082"

# Run tests with coverage
TEST_RESULTS=0

echo -e "\n${YELLOW}[1/3] Running blockchain integration tests...${NC}"
if go test -v -timeout "$TEST_TIMEOUT" ./tests/integration/blockchain_test.go ./tests/integration/helper.go; then
    echo -e "${GREEN}✓ Blockchain tests passed${NC}"
else
    echo -e "${RED}✗ Blockchain tests failed${NC}"
    TEST_RESULTS=1
fi

echo -e "\n${YELLOW}[2/3] Running API integration tests...${NC}"
if go test -v -timeout "$TEST_TIMEOUT" ./tests/integration/api_test.go ./tests/integration/helper.go; then
    echo -e "${GREEN}✓ API tests passed${NC}"
else
    echo -e "${RED}✗ API tests failed${NC}"
    TEST_RESULTS=1
fi

echo -e "\n${YELLOW}[3/3] Running WebSocket integration tests...${NC}"
if go test -v -timeout "$TEST_TIMEOUT" ./tests/integration/websocket_test.go ./tests/integration/helper.go; then
    echo -e "${GREEN}✓ WebSocket tests passed${NC}"
else
    echo -e "${RED}✗ WebSocket tests failed${NC}"
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
