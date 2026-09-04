# NexusGuard 🛡️
A high-performance Telegram Bot written in Go, designed for administration or guarding communities.

## Features
- **Telegram Bot API**: Uses `telebot.v3` for fast and reliable bot interactions.
- **PostgreSQL Database**: Uses `pgx/v5` driver for high-performance database queries and pooling.
- **Dockerized**: Fully containerized setup with `Dockerfile` and `docker-compose.yml` for easy deployment.
- **Make Automation**: Includes a `Makefile` for streamlined building and testing.

## Tech Stack
- **Language**: Go 1.25+
- **Database**: PostgreSQL
- **Libraries**: `telebot.v3`, `pgx/v5`

## Getting Started
1. Clone the repo.
2. Copy `.env.example` to `.env` and configure your Bot Token and DB credentials.
3. Run with Docker:
   ```bash
   docker-compose up -d
   ```
