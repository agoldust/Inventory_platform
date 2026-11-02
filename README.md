# Inventory Platform

Track products and their stock. A Go REST API on PostgreSQL, a Vue 3 + TypeScript frontend, Prometheus + Grafana monitoring, and a cookie-based A/B test, all started with one Docker Compose command.

![Go](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![Vue.js](https://img.shields.io/badge/Vue.js-4FC08D?style=for-the-badge&logo=vuedotjs&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-3178C6?style=for-the-badge&logo=typescript&logoColor=white)
![Vite](https://img.shields.io/badge/Vite-646CFF?style=for-the-badge&logo=vite&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white)
![nginx](https://img.shields.io/badge/nginx-009639?style=for-the-badge&logo=nginx&logoColor=white)
![Prometheus](https://img.shields.io/badge/Prometheus-E6522C?style=for-the-badge&logo=prometheus&logoColor=white)
![Grafana](https://img.shields.io/badge/Grafana-F46800?style=for-the-badge&logo=grafana&logoColor=white)

## Quick start

You need Docker. Then:

```bash
docker compose up -d --build --wait
```

| Open | URL |
|---|---|
| App | http://localhost:3000 |
| API | http://localhost:3000/api (for example `/api/products`) |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3001 (login `admin` / `admin`) |
| Postgres | `localhost:5432`, user, password and database all `inventory` |

Stop it with `docker compose down`. Add `-v` to also delete the database and the monitoring data.

Always start the whole stack with the command above. Rebuilding only some services stops the ones that depend on them.

## Try the API

```bash
# Create a product (stock starts at 0)
curl -X POST http://localhost:3000/api/products -d '{"name":"Pen","price":1.5}'
# {"id":8,"name":"Pen","price":1.5,"stock":0,...}

# Add 10 units, then try to remove 99
curl -X PATCH http://localhost:3000/api/products/8/stock -d '{"change":10,"reason":"restock"}'
curl -X PATCH http://localhost:3000/api/products/8/stock -d '{"change":-99,"reason":"sale"}'
# {"error":"insufficient stock"}   (HTTP 409)

# List, then delete
curl http://localhost:3000/api/products
curl -X DELETE http://localhost:3000/api/products/8
```

Use the `id` the first command returns. On Windows PowerShell, type `curl.exe` instead of `curl`.

## Architecture

```mermaid
flowchart LR
    user([Browser]) -->|":3000"| nginx["nginx<br/>Vue 3 + TypeScript app"]
    nginx -->|"/api/*"| api["Go API<br/>net/http + pgx"]
    api --> db[("PostgreSQL 16")]
    migrate["migrate<br/>(runs once)"] -->|"creates tables"| db
    prom["Prometheus"] -->|"scrapes /metrics"| api
    grafana["Grafana"] -->|"queries"| prom
```

nginx serves the app and forwards `/api/*` to the API without the prefix, so `/api/products` reaches the API as `/products`. The API has no port of its own on your machine. The `migrate` container creates the tables before the API starts.

## API

### Products

| Endpoint | What it does |
|---|---|
| `GET /products` | List all products |
| `GET /products/{id}` | One product, or 404 |
| `POST /products` | Create one from `{"name","price"}`. Stock starts at 0 |
| `PATCH /products/{id}/stock` | Add or remove stock with `{"change","reason"}`. Negative removes. Returns 409 if stock would go below 0 |
| `DELETE /products/{id}` | Delete a product and its stock history |

Every stock change runs in one transaction that also writes a row to `stock_movements`, so the stock level and its history always match:

```mermaid
sequenceDiagram
    participant B as Browser
    participant A as Go API
    participant D as PostgreSQL
    B->>A: PATCH /products/1/stock
    A->>D: BEGIN, UPDATE products SET stock = stock + change
    alt stock would go below 0
        D-->>A: check_violation
        A-->>B: 409 insufficient stock
    else enough stock
        A->>D: INSERT INTO stock_movements, COMMIT
        A-->>B: 200 updated product
    end
```

### Health and metrics

| Endpoint | What it does |
|---|---|
| `GET /healthz` | Returns `{"status":"ok"}` while the API runs. It does not check the database |
| `GET /metrics` | Prometheus metrics, see [Monitoring](#monitoring) |

### Experiments

| Endpoint | What it does |
|---|---|
| `GET /variant` | Returns this browser's A/B variant, and assigns one on the first visit. See [A/B test](#ab-test) |

## Monitoring

Prometheus scrapes the API every 10 seconds. Grafana starts with Prometheus already set up as its data source. Check that scraping works at http://localhost:9090/targets.

| Metric | Labels | Meaning |
|---|---|---|
| `http_requests_total` | `route`, `status`, `variant` | Requests handled |
| `http_request_duration_seconds` | `route`, `variant` | Latency histogram |
| `stock_changes_total` | `direction` (`increase` / `decrease`) | Successful stock changes |

`route` is the pattern, like `GET /products/{id}`, not the real path, so each product id doesn't create its own series. Queries to paste into Prometheus or Grafana:

```promql
# Requests per second, per endpoint
sum by (route) (rate(http_requests_total[1m]))

# 95th percentile latency, per endpoint
histogram_quantile(0.95, sum by (le, route) (rate(http_request_duration_seconds_bucket[5m])))
```

## A/B test

Each browser gets variant **A** (table layout) or **B** (card grid) at random and keeps it for a year in an `ab_variant` cookie. Every API request is labelled with its variant, so you can compare the two:

```promql
sum by (variant) (rate(http_requests_total{route="GET /products"}[5m]))
```

```mermaid
flowchart TD
    load["Page loads"] --> ask["GET /variant"]
    ask --> has{"Valid ab_variant cookie<br/>(A or B)?"}
    has -->|yes| keep["Keep the variant"]
    has -->|no| assign["Pick A or B at random (50/50)<br/>Set-Cookie for 1 year"]
    keep --> pick{"Variant"}
    assign --> pick
    pick -->|A| tbl["Table layout"]
    pick -->|B| cards["Card grid layout"]
    tbl --> req["Later requests send the cookie"]
    cards --> req
    req --> metrics["API labels each request<br/>variant = A or B"]
    metrics --> graf["Prometheus and Grafana<br/>compare A with B"]
```

Requests without the cookie, like Prometheus scrapes or `curl`, are labelled `none`. To see the other layout, delete the cookie, or open the app at `127.0.0.1:3000` instead of `localhost:3000`.

## Database

Two tables, created by [migrations/](migrations/):

- `products`: `id`, `name`, `price` (≥ 0), `stock` (≥ 0), `created_at`, `updated_at`
- `stock_movements`: `id`, `product_id` (deleted with its product), `change` (never 0), `reason`, `created_at`

The database itself enforces the "never below 0" rule, so concurrent requests can't oversell. Look inside with:

```bash
docker compose exec postgres psql -U inventory -d inventory -c "\dt"
```

## Development

Run the API and frontend on your machine, with only the database in Docker. Use three terminals, starting from the project folder in each:

```bash
# 1. Database and tables
docker compose up -d postgres migrate

# 2. API on http://localhost:8080
cd api
export DATABASE_URL="postgres://inventory:inventory@localhost:5432/inventory?sslmode=disable"
go run .

# 3. Frontend on http://localhost:5173 (forwards /api to :8080)
cd frontend
npm install
npm run dev
```

On Windows PowerShell, set the variable with `$env:DATABASE_URL="postgres://..."` instead of `export`. The API also reads `PORT` (default 8080).

Tests and checks:

```bash
cd api && go test ./...          # API tests
cd frontend && npm run build     # TypeScript type check and build
```

## Project layout

| Folder | What's in it | Built with |
|---|---|---|
| [api/](api/) | REST API, metrics, A/B variant, tests | Go 1.22+ `net/http`, pgx/v5, prometheus/client_golang |
| [frontend/](frontend/) | The web app, its Dockerfile and nginx config | Vue 3, TypeScript, Vite, nginx |
| [migrations/](migrations/) | Table definitions | SQL, golang-migrate |
| [monitoring/](monitoring/) | Prometheus and Grafana config | |

## Known limits

- No login, no pagination. `/metrics` is open to anyone who can reach the app.
- The passwords in `docker-compose.yml` are local defaults. Change them before deploying anywhere.
- Prices are `float64` in Go. Switch to a decimal type before doing arithmetic on money.
