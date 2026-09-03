# NexusGuard 🛡️

NexusGuard is a robust Telegram bot built with **Go** using the Clean Architecture pattern. It utilizes PostgreSQL for database operations and supports graceful shutdown and proxy configuration.

## Features

- **Clean Architecture:** Organized structure (`cmd`, `internal`, `pkg`) for maintainability.
- **PostgreSQL:** Reliable database integration using `database/sql`.
- **Proxy Support:** Ready for restricted network environments.
- **Dockerized:** Simple deployment using `docker-compose`.
- **Graceful Shutdown:** Safe handling of OS signals to gracefully stop operations.

## Prerequisites

- [Go](https://golang.org/doc/install) (1.25 or later)
- [Docker & Docker Compose](https://docs.docker.com/get-docker/)

## Quick Start

1. **Clone the repository:**
   ```bash
   git clone https://github.com/VaFa1726/NexusGuard.git
   cd NexusGuard
   ```

2. **Configuration:**
   Copy the example environment file and add your configuration details.
   ```bash
   cp .env.example .env
   ```
   *Edit `.env` and add your `TELEGRAM_TOKEN`.*

3. **Start the Database:**
   ```bash
   make docker-up
   # Or manually: docker compose up -d postgres
   ```

4. **Run the Bot:**
   ```bash
   make run
   ```

## Project Structure

```
├── cmd/bot           # Application entry point
├── internal/         # Private application and library code (Clean Architecture)
│   ├── delivery      # HTTP/Bot handlers
│   ├── usecase       # Business logic
│   ├── repository    # Database/API access
│   └── domain        # Business domain models
├── pkg/              # Public library code (Logger, Database, Config)
├── Dockerfile        # Docker setup for the bot
├── docker-compose.yml# Multi-container orchestration
└── Makefile          # Useful commands for development
```

## Contributing

Contributions are welcome! Feel free to open an issue or submit a pull request.

## License

MIT License
