# Inventory Platform

A small inventory management app: a Go REST API on PostgreSQL, a Vue 3 + TypeScript frontend, and Prometheus + Grafana monitoring. Everything runs with Docker Compose.

![Go](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![Vue.js](https://img.shields.io/badge/Vue.js-4FC08D?style=for-the-badge&logo=vuedotjs&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-3178C6?style=for-the-badge&logo=typescript&logoColor=white)
![Vite](https://img.shields.io/badge/Vite-646CFF?style=for-the-badge&logo=vite&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white)
![nginx](https://img.shields.io/badge/nginx-009639?style=for-the-badge&logo=nginx&logoColor=white)
![Prometheus](https://img.shields.io/badge/Prometheus-E6522C?style=for-the-badge&logo=prometheus&logoColor=white)
![Grafana](https://img.shields.io/badge/Grafana-F46800?style=for-the-badge&logo=grafana&logoColor=white)

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

| Part | Tech |
|---|---|
| API | Go 1.22+ (`net/http` routing), pgx/v5, `prometheus/client_golang` |
| Database | PostgreSQL 16, migrations in [golang-migrate](https://github.com/golang-migrate/migrate) format |
| Frontend | Vue 3, TypeScript, Vite, served by nginx |
| Monitoring | Prometheus, Grafana |
| Packaging | Docker Compose, multi-stage Dockerfiles |

## Run it

```bash
docker compose up -d --build --wait
```

| Service | URL |
|---|---|
| App | http://localhost:3000 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3001 (login `admin` / `admin`, Prometheus is preconfigured) |
| Postgres | `localhost:5432`, user, password and database all `inventory` |

A one-shot `migrate` container creates the tables before the API starts. Always start the whole stack as above: rebuilding only some services stops the ones that depend on them.

## API

The browser talks to the API through nginx under `/api` (for example `/api/products`).

| Method and path | Description |
|---|---|
| `GET /healthz` | Liveness check |
| `GET /products` | List products |
| `GET /products/{id}` | One product, 404 if missing |
| `POST /products` | Create `{"name","price"}`; stock starts at 0 |
| `PATCH /products/{id}/stock` | Change stock with `{"change","reason"}` (negative removes). Runs in one transaction that also writes a `stock_movements` row. Returns 409 if stock would go negative |
| `DELETE /products/{id}` | Delete a product and its stock history (cascade) |
| `GET /variant` | Returns the caller's A/B variant, assigning one by cookie on the first visit |
| `GET /metrics` | Prometheus metrics |

### Stock update flow

```mermaid
sequenceDiagram
    participant B as Browser
    participant N as nginx
    participant A as Go API
    participant D as PostgreSQL
    participant P as Prometheus
    B->>N: PATCH /api/products/1/stock
    N->>A: PATCH /products/1/stock
    A->>D: BEGIN, UPDATE products SET stock = stock + change
    alt stock would go below 0
        D-->>A: check_violation
        A-->>B: 409 insufficient stock
    else enough stock
        A->>D: INSERT INTO stock_movements, COMMIT
        A-->>B: 200 updated product
        Note over A: stock_changes_total +1
    end
    P->>A: scrape /metrics every 10s
```

## Database

Two tables, created by the files in [migrations/](migrations/):

- `products`: `id`, `name`, `price NUMERIC(10,2) >= 0`, `stock INTEGER >= 0`, `created_at`, `updated_at`
- `stock_movements`: `id`, `product_id` (foreign key, `ON DELETE CASCADE`, indexed), `change <> 0`, `reason`, `created_at`

Check them with psql:

```bash
docker compose exec postgres psql -U inventory -d inventory -c "\dt"
```

## Metrics

- `http_requests_total{route,status,variant}`: request count
- `http_request_duration_seconds{route,variant}`: latency histogram
- `stock_changes_total{direction}`: successful stock changes (`increase` / `decrease`)

`route` is the route pattern (`GET /products/{id}`), not the raw path, so the number of time series stays bounded. Example queries:

```promql
sum by (route) (rate(http_requests_total[1m]))
histogram_quantile(0.95, sum by (le, route) (rate(http_request_duration_seconds_bucket[5m])))
```

## A/B test

Each browser is assigned variant **A** (table layout) or **B** (card grid) at random, stored in an `ab_variant` cookie for a year. The frontend asks `GET /variant` before loading products, and every request is labelled with its variant in the metrics above, for example:

```promql
sum by (variant) (rate(http_requests_total{route="GET /products"}[5m]))
```

Requests without a cookie, such as Prometheus scraping `/metrics`, are labelled `none`. To see the other layout, delete the cookie, or open the app on `127.0.0.1` instead of `localhost`.

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

## Develop without Docker for the app

```bash
docker compose up -d postgres migrate          # database + tables

cd api
DATABASE_URL="postgres://inventory:inventory@localhost:5432/inventory?sslmode=disable" go run .
go test ./...

cd frontend
npm install
npm run dev                                    # http://localhost:5173, proxies /api to :8080
```

The API reads `DATABASE_URL` (required) and `PORT` (default 8080).

## Project layout

```
api/         Go API (handlers, Store interface + pgx store, metrics, A/B variant, tests)
frontend/    Vue 3 + TypeScript app, Dockerfile, nginx.conf
migrations/  SQL migrations (golang-migrate)
monitoring/  Prometheus config and Grafana data source
```

## Known limits

- `price` is a `float64` in the API: fine for display, but use a decimal type before doing arithmetic on money.
- There is no authentication, pagination, or graceful shutdown.
- Credentials in `docker-compose.yml` are local development defaults. Move them to an `.env` file and change them before deploying anywhere.
- `/metrics` is unauthenticated. nginx doesn't expose it, but the API does inside the compose network.
