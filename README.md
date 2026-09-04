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
- **Telegram Bot API Integration**: Fast and reliable bot interactions.
- **Database Connection Pooling**: Optimized queries using `pgx/v5`.
- **Dockerized Environment**: Fully containerized setup for seamless deployment.
- **Make Automation**: Streamlined building and testing via `Makefile`.

## 🛠️ Prerequisites
- Go 1.25 or higher
- Docker and Docker Compose
- PostgreSQL (if running locally without Docker)

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
