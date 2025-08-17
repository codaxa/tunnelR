env "development" {
  src = "postgres://${getenv("DB_USER")}:${getenv("DB_PASSWORD")}@${getenv("DB_HOST")}:5432/${getenv("DB_NAME")}_temp?sslmode=disable"
  dev = "docker://postgres/15/dev"  url = "postgres://${getenv("DB_USER")}:${getenv("DB_PASSWORD")}@${getenv("DB_HOST")}:5432/${getenv("DB_NAME")}?sslmode=disable"
  migration {
    dir = "file://internal/api/infrastructure/db/migrations"
  }
}