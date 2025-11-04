# -------------------------
# Standalone Next.js Admin Web Application
# -------------------------
# This Dockerfile builds a standalone container for the admin web UI
# that communicates with the admin API (Go backend).
#
# Build args:
#   - NEXT_PUBLIC_API_BASE: API endpoint (e.g., http://localhost:3000 for dev)
#
# The container runs Next.js in production mode and serves on port 3003.
# -------------------------

# -------------------------
# 1) Build stage
# -------------------------
FROM node:22-alpine AS builder
WORKDIR /app

# Enable corepack (ships with Node 22)
RUN corepack enable

# Copy package manager files for better caching
COPY package.json pnpm-lock.yaml* .npmrc* ./

# Install dependencies
RUN pnpm install --frozen-lockfile

# Copy source files
COPY . .

# Build the Next.js application
RUN pnpm build

# -------------------------
# 2) Production runtime
# -------------------------
FROM node:22-alpine AS runner
WORKDIR /app

# Enable corepack
RUN corepack enable

# Don't run as root
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

# Set ownership of /app to nextjs user before copying files
RUN chown -R nextjs:nodejs /app

# Switch to nextjs user for all subsequent operations
USER nextjs

# Copy package files and install ALL dependencies (needed for dev mode)
COPY --chown=nextjs:nodejs --from=builder /app/package.json /app/pnpm-lock.yaml* ./
RUN pnpm install --frozen-lockfile

# Copy built application
COPY --chown=nextjs:nodejs --from=builder /app/.next ./.next
COPY --chown=nextjs:nodejs --from=builder /app/app ./app
COPY --chown=nextjs:nodejs --from=builder /app/public ./public
COPY --chown=nextjs:nodejs --from=builder /app/next.config.js ./
COPY --chown=nextjs:nodejs --from=builder /app/tailwind.config.js ./
COPY --chown=nextjs:nodejs --from=builder /app/postcss.config.js ./
COPY --chown=nextjs:nodejs --from=builder /app/tsconfig.json ./
COPY --chown=nextjs:nodejs --from=builder /app/package.json ./

# Expose the port Next.js will run on
EXPOSE 3003

# Set environment variables
ENV PORT=3003
ENV HOSTNAME="0.0.0.0"
# Disable Next.js telemetry to avoid permission issues
ENV NEXT_TELEMETRY_DISABLED=1

# Start Next.js - supports both dev and production modes
# Use NODE_ENV to control: development = hot reload, production = optimized build
CMD ["sh", "-c", "if [ \"$NODE_ENV\" = \"development\" ]; then pnpm dev; else pnpm start; fi"]
