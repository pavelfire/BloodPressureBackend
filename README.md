# BloodPressureBackend

REST API для приложения BloodPressureNotes и будущего веб-сайта.

## Стек

- Go 1.22+ (`net/http`, без веб-фреймворков)
- PostgreSQL
- JWT (HMAC-SHA256, stdlib) + refresh-токены
- bcrypt для паролей

## Быстрый старт

```bash
cd BloodPressureBackend
docker compose up -d postgres
go mod tidy
go run ./cmd/api
```

API: `http://localhost:8080`  
Health: `GET /health`

Для Android-эмулятора: `http://10.0.2.2:8080`

## Переменные окружения

Скопируйте `.env.example` и задайте значения:

| Переменная | Описание |
|------------|----------|
| `DATABASE_URL` | строка подключения PostgreSQL |
| `JWT_SECRET` | секрет для access-токенов |
| `PORT` | порт HTTP (по умолчанию 8080) |
| `CORS_ORIGIN` | origin для CORS (`*` для разработки) |

## API (`/api/v1`)

### Auth

- `POST /auth/register` — `{ "email", "password" }` (пароль ≥ 8 символов)
- `POST /auth/login`
- `POST /auth/refresh` — `{ "refreshToken" }`
- `POST /auth/logout` — `{ "refreshToken" }`
- `GET /auth/me` — Bearer token

### Readings (требуют Bearer token)

- `GET /readings?since=<RFC3339>&limit=100`
- `GET /readings/{id}`
- `POST /readings`
- `PUT /readings/{id}`
- `DELETE /readings/{id}`
- `POST /sync` — batch-синхронизация для offline-first клиента

## Docker (полный стек)

```bash
docker compose up --build
```

## Пример

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"password123"}'
```
