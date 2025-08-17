# CanopyX Admin UI (Next.js 15, App Router)

- Next.js **15** with **App Router** (CSR + static export)
- TailwindCSS styling
- Served by the Go manager at `/admin/*`

## Dev
```
cd web/admin
pnpm i
pnpm dev -p 5173
# open http://localhost:5173/admin
```

## Build & embed into Go
```
cd web/admin
pnpm i
pnpm build   # static export emitted because next.config.js has output: 'export'
# rebuild manager to embed static assets
go build ./cmd/manager
```
