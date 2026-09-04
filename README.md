<div align="center">
  <h1>NexusGuard 🛡️</h1>
  <p>A high-performance Telegram Bot written in Go, designed for administration and monitoring.</p>
  
  ![Go](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)
  ![PostgreSQL](https://img.shields.io/badge/PostgreSQL-316192?style=for-the-badge&logo=postgresql&logoColor=white)
  ![Docker](https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white)
</div>

---

## 📌 Overview
NexusGuard is a robust Telegram bot application built with Go. It leverages the `telebot.v3` framework for interacting with the Telegram API and connects to a PostgreSQL database using `pgx/v5` for high-performance data operations. 

## ✨ Features
- **Telegram Bot API Integration**: Fast and reliable bot interactions with rate limiting
- **Database Connection Pooling**: Optimized for 1000+ concurrent users using `pgx/v5`
- **Worker Pool Architecture**: 10 parallel workers with batch processing for high throughput
- **Rate Limiting**: Per-user rate limiting to prevent abuse and spam (10 req/sec)
- **Concurrent Operations**: Goroutine pools for efficient API calls
- **Health Monitoring**: Built-in health check and metrics endpoints
- **Graceful Shutdown**: Proper cleanup of all resources
- **Automated Cleanup**: Periodic database maintenance and memory management
- **Unit Tests**: Test coverage for critical components
- **Dockerized Environment**: Fully containerized setup for seamless deployment
- **Make Automation**: Streamlined building and testing via `Makefile`

## 🚀 Performance Highlights
- **Database Pool**: 100 max connections (10x increase)
- **XP Processing**: 3000+ events/sec with 10 workers
- **Queue Capacity**: 5000 events buffer
- **Rate Limiting**: 4M checks/sec per-user protection
- **Concurrent Groups**: 10 parallel Telegram API calls
- **Health Checks**: `/health`, `/ready`, `/metrics` endpoints on port 8080

## 🛠️ Prerequisites
- Go 1.25 or higher
- Docker and Docker Compose
- PostgreSQL (if running locally without Docker)

## 📊 Performance & Scalability
- **Designed for 1000+ concurrent users**
- Comprehensive performance benchmarks in [docs/PERFORMANCE.md](docs/PERFORMANCE.md)
- Built-in monitoring and health checks
- See [Performance Report](docs/PERFORMANCE.md) for detailed metrics

## 🚀 Installation & Usage

1. **Clone the repository:**
   ```bash
   git clone https://github.com/VaFa1726/NexusGuard.git
   cd NexusGuard
   ```

2. **Environment Setup:**
   Copy the `.env.example` file to `.env` and fill in your Telegram Bot Token and Database credentials.
   ```bash
   cp .env.example .env
   ```

3. **Run with Docker (Recommended):**
   ```bash
   docker-compose up -d --build
   ```

4. **Build Locally (Using Makefile):**
   ```bash
   make build
   make run
   ```

## 🧪 Testing
Run unit tests:
```bash
make test
```

Run with race detection:
```bash
go test ./... -race
```

Run benchmarks:
```bash
go test ./pkg/ratelimit -bench=. -benchmem
go test ./internal/usecase -bench=. -benchmem
```

## 📈 Monitoring
Access health check endpoints:
```bash
# Health status
curl http://localhost:8080/health

# Readiness check
curl http://localhost:8080/ready

# Detailed metrics
curl http://localhost:8080/metrics
```

## 📂 Project Architecture
Following Go standard project layout:
```text
NexusGuard/
├── cmd/               # Main applications for this project
├── internal/          # Private application and library code
├── pkg/               # Library code that's ok to use by external applications
├── docker-compose.yml # Container orchestration
└── Makefile           # Build automation
```
