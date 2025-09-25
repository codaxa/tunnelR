# tunnelR

A cross-platform CLI for creating, managing, and revoking time-limited SSH access to Linux servers with Zero Trust and Just-in-Time (JIT) security principles.

## Overview

tunnelR ensures users get the minimum required access for the shortest necessary time through:

- **Zero Trust Architecture**: Every access request is authenticated and authorized
- **Just-in-Time Access**: Temporary SSH access that automatically expires
- **Centralized Management**: Role-based access control through a backend API
- **Audit Trail**: Complete logging of all SSH access events

## Architecture

The project consists of two main components:

### 1. CLI Application (`cmd/cli/`)
- **Connect**: Initiate connections to backend servers
- **Login**: Authenticate with the backend API
- **Configuration Management**: Store server and authentication settings

### 2. Backend API (`cmd/api/`)
- **User Management**: Register and authenticate users
- **Role-Based Access**: Admin, Operator, and Read-only roles
- **JWT Authentication**: Secure token-based authentication
- **PostgreSQL Storage**: Persistent user and access data

## Quick Start

### Prerequisites
- Go 1.24+
- PostgreSQL 15+
- [Task](https://taskfile.dev/) (optional, for development)

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/codaxa/tunnelR.git
   cd tunnelR
   ```

2. **Set up environment variables**
   ```bash
   cp .env.example .env
   # Edit .env with your configuration
   ```

3. **Build both applications**
   ```bash
   task build
   # Or manually:
   # go build -o bin/tunnelr-cli ./cmd/cli/main.go
   # go build -o bin/tunnelr-api ./cmd/api/main.go
   ```

### Database Setup

1. **Create and migrate database**
   ```bash
   task setup
   ```

2. **Or manually**
   ```bash
   task db-create
   task migrate-init
   task migrate-up
   ```

## Usage

### Starting the Backend API

```bash
# Development with hot reload
task dev-air

# Or run directly
task run-api
# API runs on http://localhost:8080
```

### Using the CLI

1. **Connect to a backend server**
   ```bash
   ./bin/tunnelr-cli connect --user alice --server localhost:8080
   ```

2. **Login and get authentication token**
   ```bash
   ./bin/tunnelr-cli login --user alice --password secret123
   ```

3. **Get help**
   ```bash
   ./bin/tunnelr-cli --help
   ./bin/tunnelr-cli connect --help
   ```

## API Endpoints

### Public Endpoints
- `GET /healthz` - Health check
- `GET /version` - API version
- `POST /api/login` - User authentication

### Admin Endpoints (Requires admin role)
- `POST /api/register` - Register new users

## Configuration

### Environment Variables

Create a `.env` file with:

```env
PORT=8080
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=tunnelr
JWT_SECRET=your_jwt_secret_key
```

### CLI Configuration

The CLI stores configuration in `~/.tunnelr/config.json`:

```json
{
  "server": "localhost:8080",
  "token": "your_jwt_token"
}
```

## User Roles

- **Admin**: Full access to user management and system configuration
- **Operator**: Can request and manage SSH access
- **Read-only**: View-only access to audit logs and status

## Development

### Available Tasks

```bash
task --list
```

Key development tasks:
- `task dev-air` - Start API with hot reload
- `task run-cli` - Run CLI application
- `task test` - Run all tests
- `task lint` - Run linter
- `task check` - Run all checks (test, lint, vet, format)

### Database Management

```bash
# Fresh database setup
task setup-fresh

# Create new migration
task migrate-new

# Check migration status
task migrate-status

# Apply migrations
task migrate-up
```

### Project Structure

```
.
├── cmd/
│   ├── api/           # Backend API server
│   └── cli/           # CLI application
├── configs/           # Configuration management
├── internal/
│   ├── api/          # Backend business logic
│   │   ├── app/      # Application services
│   │   ├── core/     # Domain models and interfaces
│   │   ├── infrastructure/ # Database and external services
│   │   └── presentation/   # HTTP handlers and middleware
│   └── cli/          # CLI infrastructure
├── pkg/              # Public packages
└── scripts/          # Utility scripts
```

## Security Features

- **JWT Authentication**: Secure token-based authentication
- **Password Hashing**: bcrypt for secure password storage
- **Role-Based Access Control**: Granular permissions system
- **Database Constraints**: SQL-level data validation
- **Environment Encryption**: GPG-encrypted environment files

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `task check`
5. Submit a pull request

## License

[Add your license here]

## Roadmap

- [ ] SSH key management
- [ ] Time-limited access controls
- [ ] Audit logging and reporting
- [ ] Multi-server support
- [ ] Web dashboard
- [ ] Integration with cloud providers

## Support

For issues and questions:
- Create an issue on GitHub
- Check the [documentation](docs/)
- Review the CLI help: `tunnelr --help`