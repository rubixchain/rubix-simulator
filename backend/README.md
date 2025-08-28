# Rubix Network Transaction Simulator - Backend

A Go-based backend service for simulating Rubix network transactions and generating comprehensive PDF reports.

## Features

- **Node Management**: Start and stop 2-20 Rubix testnet nodes
- **Transaction Simulation**: Execute token transfers across nodes
- **Real-time Monitoring**: Track simulation progress with live updates
- **PDF Reports**: Generate detailed reports with charts and statistics
- **RESTful API**: Clean API for frontend integration

## Project Structure

```
backend/
├── cmd/
│   └── server/         # Application entry point
├── internal/
│   ├── config/         # Configuration management
│   ├── handlers/       # HTTP request handlers
│   ├── middleware/     # HTTP middleware
│   ├── models/         # Data structures
│   └── services/       # Business logic
│       ├── node_manager.go         # Node lifecycle management
│       ├── transaction_executor.go # Transaction execution
│       ├── report_generator.go     # PDF generation
│       └── simulation_service.go   # Simulation orchestration
├── reports/            # Generated PDF reports
└── scripts/            # Helper scripts

```

## Installation

### Prerequisites

- Go 1.21 or higher
- Python (if using real Rubix nodes)
- Rubix testnet scripts (optional)

### Setup

1. Clone the repository
2. Install Go dependencies:
```bash
cd backend
go mod download
```

3. Build the application:
```bash
go build -o rubix-simulator ./cmd/server
```

## Configuration

Set environment variables to configure the service:

```bash
# Server port (default: 8080)
export PORT=8080

# Path to Rubix Python script (optional)
export RUBIX_SCRIPT_PATH=/path/to/rubix-testnet-script.py

# Reports directory (default: ./reports)
export REPORTS_PATH=./reports
```

## Running the Server

### Development Mode
```bash
go run cmd/server/main.go
```

### Production Mode
```bash
./rubix-simulator
```

The server will start on `http://localhost:8080`

## API Endpoints

### Simulation Control

#### Start Simulation
```http
POST /simulate
Content-Type: application/json

{
  "nodes": 5,
  "transactions": 100
}

Response:
{
  "simulationId": "uuid",
  "message": "Simulation started successfully"
}
```

#### Get Simulation Status
```http
GET /report/{simulationId}

Response:
{
  "simulationId": "uuid",
  "transactionsCompleted": 50,
  "totalTransactions": 100,
  "successCount": 45,
  "failureCount": 5,
  "averageLatency": 250.5,
  "isFinished": false,
  ...
}
```

### Reports

#### Download PDF Report
```http
GET /reports/{simulationId}/download

Returns: PDF file
```

#### List Available Reports
```http
GET /reports/list

Response:
[
  {
    "id": "simulation-uuid",
    "filename": "simulation-uuid.pdf",
    "createdAt": "2024-01-15T10:30:00Z",
    "size": 245632
  }
]
```

### Node Management

#### Start Nodes
```http
POST /nodes/start
{
  "count": 5
}
```

#### Stop Nodes
```http
POST /nodes/stop
```

### Health Check
```http
GET /health

Response:
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "version": "1.0.0"
}
```

## PDF Report Contents

Generated reports include:

1. **Summary Section**
   - Total nodes and transactions
   - Success/failure rates
   - Latency statistics
   - Token transfer totals
   - Execution time

2. **Node Performance Breakdown**
   - Transactions per node
   - Success rates by node
   - Average latency per node

3. **Transaction Log**
   - Detailed transaction records
   - Status, timing, and error messages

4. **Visual Charts**
   - Success/failure pie chart
   - Latency distribution histogram
   - Node load distribution

## Simulation Modes

### With Real Rubix Nodes
Set `RUBIX_SCRIPT_PATH` to use actual Rubix testnet nodes:
- Starts Python scripts for each node
- Executes real RBT transfers
- Captures actual network metrics

### Simulated Mode (Default)
When no script path is configured:
- Generates realistic test data
- Simulates network behavior
- Useful for development/testing

## Development

### Running Tests
```bash
go test ./...
```

### Adding New Features

1. **Models**: Define data structures in `internal/models/`
2. **Services**: Implement business logic in `internal/services/`
3. **Handlers**: Add API endpoints in `internal/handlers/`
4. **Middleware**: Add cross-cutting concerns in `internal/middleware/`

## Error Handling

The service includes comprehensive error handling:
- Node startup failures
- Transaction timeouts
- Network errors
- PDF generation issues

All errors are logged and returned with appropriate HTTP status codes.

## Performance Considerations

- Transactions execute in parallel with goroutines
- Semaphore limits concurrent requests (default: 10)
- Real-time updates via polling (2-second intervals)
- PDF generation runs asynchronously

## Future Enhancements

- Database persistence (SQLite/PostgreSQL)
- WebSocket support for real-time updates
- Historical report analytics
- Custom transaction scenarios
- Prometheus metrics integration

## License

MIT