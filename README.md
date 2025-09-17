# Rubix Network Simulator

A comprehensive testing and analysis tool for the Rubix blockchain network that simulates real blockchain transactions, measures performance metrics, and generates detailed reports.

## 🚀 Quick Start

### One-Command Start (Recommended)

#### Windows
```bash
start-all.bat
```

#### Linux/Mac
```bash
chmod +x start-all.sh
./start-all.sh
```

This will automatically:
- Start the Go backend server on port 8080 
- Start the Vite development server (React + TypeScript) on port 5173
- Open your browser to http://localhost:5173

To stop all services, press `Ctrl+C` in the terminal or close the command windows.

## 📋 Prerequisites

- **Node.js** (v18 or higher) - [Download](https://nodejs.org/)
- **Go** (v1.21 or higher) - [Download](https://go.dev/dl/)
- **Git** - [Download](https://git-scm.com/)
- **Build tools**:
  - Windows: MinGW or Visual Studio Build Tools
  - Linux: gcc, make
  - Mac: Xcode Command Line Tools

### System Requirements
- **OS**: Windows 10/11, Ubuntu 20.04+, macOS 11+
- **RAM**: Minimum 8GB (16GB recommended for 20+ nodes)
- **Storage**: 5GB free space
- **Ports**: 8080, 5173, 20000-20030, 10500-10530 must be available

## 🛠️ Manual Installation

If you prefer to set up manually or the start-all script doesn't work:

### 1. Clone the Repository
```bash
git clone <your-repository-url>
cd rubix-simulator
```

### 2. Install Frontend Dependencies
```bash
npm install
```

### 3. Install Backend Dependencies
```bash
cd backend
go mod download
cd ..
```

### 4. Start Services Separately

**Terminal 1 - Backend:**
```bash
# Option 1: Use helper script
./run-backend.sh      # Linux/Mac
run-backend.bat       # Windows

# Option 2: Manual start
cd backend
go run cmd/server/main.go
```

**Terminal 2 - Frontend:**
```bash
# Option 1: Use helper script
./run-frontend.sh     # Linux/Mac
run-frontend.bat      # Windows

# Option 2: Manual start
npm run dev
```

## 🎮 Using the Simulator

### Understanding the Network

The simulator creates a Rubix blockchain network with two types of nodes:

- **Quorum Nodes (7)**: Fixed consensus nodes that validate all transactions
- **Transaction Nodes (2-20)**: Variable nodes that perform RBT token transfers

### Running Simulations

1. **Open the Application**
   - Navigate to http://localhost:5173
   - Verify "Backend Connected" badge is green

2. **Configure Simulation Parameters**
   - **Transaction Nodes**: Enter 2-20 (these are added to the 7 quorum nodes)
   - **Number of Transactions**: Enter 1-500
   - Example: 3 transaction nodes = 10 total nodes (7 quorum + 3 transaction)

3. **Start Simulation**
   - Click "Start Simulation" button
   - First run takes 5-10 minutes (downloads and builds Rubix platform)
   - Subsequent runs are faster (nodes remain running)

4. **Monitor Progress**
   - Real-time progress bar shows completed transactions
   - Live metrics update as transactions complete
   - Transaction details display with status and timing

5. **View Results**
   - Success/failure counts with percentage
   - Transaction timing (min/avg/max)
   - Total tokens transferred
   - Per-node performance breakdown

6. **Download Report**
   - Click "Download PDF Report" for comprehensive analysis
   - Includes charts, graphs, and detailed transaction logs

### Node Management

**Important**: Nodes remain running between simulations for faster testing.

- **Shutdown Nodes**: Click "Shutdown All Nodes" button when finished testing
- **Script Shutdown**: Use `./shutdown-nodes.sh` (Linux/Mac) or `shutdown-nodes.bat` (Windows)
- **Auto-cleanup**: Nodes automatically shut down when backend stops (Ctrl+C)
- **Fresh Start**: Shutdown nodes → Start new simulation

## 📊 Understanding Results

### Transaction Flow

1. **Pairing**: Nodes are paired (sender/receiver) to prevent conflicts
2. **Token Generation**: Each sender creates 10 RBT tokens
3. **Transfer**: Tokens sent via Rubix blockchain protocol
4. **Validation**: Quorum nodes reach consensus
5. **Confirmation**: Blockchain confirms transaction completion

### Performance Metrics

- **Fast**: < 1 minute (small amounts, optimal conditions)
- **Normal**: 1-3 minutes (typical transactions)
- **Slow**: 3-6+ minutes (large amounts or network congestion)

### PDF Report Contents

- Executive summary with key metrics
- Transaction timeline visualization
- Success/failure distribution charts
- Token amount vs. time correlation
- Node performance comparison
- Complete transaction logs with IDs

## 🐛 Troubleshooting

### Backend Connection Issues
```
Symptom: "Backend Offline" badge in UI
```
**Solutions:**
- Ensure backend is running (`go run cmd/server/main.go`)
- Check port 8080 is not in use: `netstat -an | findstr 8080`
- Verify firewall settings allow localhost connections

### Node Startup Failures
```
Error: "Failed to start Rubix nodes"
```
**Solutions:**
- Check disk space (need ~500MB per node)
- Ensure required ports are free (20000-20030, 10500-10530)
- Delete `backend/rubix-data` folder and retry
- Run with administrator/sudo privileges if needed

### Transaction Failures
```
Symptom: High transaction failure rate
```
**Solutions:**
- Ensure minimum 2 transaction nodes configured
- Check backend logs for specific errors
- Shutdown and restart nodes if running for extended period
- Verify network connectivity


## 📁 Project Structure

```
rubix-simulator/
├── backend/                 # Go backend server
│   ├── cmd/
│   │   ├── server/          # Server entry point
│   │   └── test_rubix/      # Test utilities
│   ├── config/             # Configuration files
│   ├── internal/            # Core business logic
│   │   ├── config/         # Config management
│   │   ├── handlers/       # HTTP request handlers
│   │   ├── middleware/     # HTTP middleware
│   │   ├── models/         # Data structures
│   │   ├── rubix/          # Rubix blockchain integration
│   │   └── services/       # Node, transaction, report services
│   ├── reports/            # Generated PDF reports
│   ├── rubix-data/         # Runtime node data (git-ignored)
│   ├── go.mod              # Go dependencies
│   └── README.md           # Backend documentation
├── src/                     # React frontend (Vite + TypeScript)
│   ├── components/         # UI components
│   │   └── ui/             # Shadcn/ui components (Radix UI)
│   ├── hooks/              # React hooks
│   ├── lib/                # Utilities
│   └── pages/              # Application pages
├── public/                  # Static assets
├── package.json            # Frontend dependencies
├── vite.config.ts          # Vite configuration
├── tailwind.config.ts      # Tailwind CSS config
├── start-all.bat           # Windows quick-start script
├── start-all.sh            # Linux/Mac quick-start script
├── run-backend.*           # Backend start scripts
├── run-frontend.*          # Frontend start scripts
├── shutdown-nodes.*        # Node shutdown scripts
└── README.md               # This file
```

## 🔧 Development

### Running Tests
```bash
# Backend tests
cd backend
go test ./...

# Frontend linting
npm run lint
```

### Code Format
```bash
# Backend formatting
cd backend
go fmt ./...

# Frontend linting and formatting
npm run lint
```

### Adding Features

1. Backend changes: Modify services in `backend/internal/`
2. Frontend changes: Update components in `src/components/`
3. API changes: Update handlers in `backend/internal/handlers/`

## 📈 Performance Optimization

### Recommended Settings

- **Quick Test**: 2 nodes, 10 transactions
- **Standard Test**: 5 nodes, 50 transactions
- **Stress Test**: 15-20 nodes, 200-500 transactions

### Resource Usage

- Each node: ~100-200MB RAM
- Backend server: ~50MB RAM
- Frontend dev server: ~100MB RAM
- Disk: ~50MB per node for blockchain data

## 📝 API Documentation

### Key Endpoints

- `POST /simulate` - Start new simulation
- `GET /report/{id}` - Get simulation status
- `GET /reports/{id}/download` - Download PDF report
- `POST /nodes/stop` - Shutdown all nodes
- `GET /health` - Backend health check

### Simulation Request
```json
{
  "nodes": 5,        // Transaction nodes (2-20)
  "transactions": 50  // Number of transactions (1-500)
}
```

## 🤝 Contributing

1. Fork the repository
2. Create feature branch (`git checkout -b feature/amazing-feature`)
3. Commit changes (`git commit -m 'Add amazing feature'`)
4. Push to branch (`git push origin feature/amazing-feature`)
5. Open Pull Request

## 📄 License

This project is licensed under the MIT License - see LICENSE file for details.

## 🆘 Support

For issues or questions:
1. Check the troubleshooting section above
2. Review backend logs in terminal
3. Check node logs in `backend/rubix-data/node*/log.txt`
4. Open an issue on GitHub with:
   - Error messages
   - Steps to reproduce
   - System information

## 🏃 Quick Commands Reference

```bash
# Start everything
./start-all.sh          # Linux/Mac
start-all.bat           # Windows

# Manual start
cd backend && go run cmd/server/main.go  # Terminal 1
npm run dev                               # Terminal 2

# Build for production
cd backend && go build -o rubix-simulator cmd/server/main.go
npm run build

# Alternative build scripts
./build.sh             # Linux/Mac
build.bat              # Windows