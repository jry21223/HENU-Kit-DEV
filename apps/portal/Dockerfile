FROM node:22-alpine AS pnpm
ENV npm_config_registry=https://registry.npmmirror.com
RUN corepack enable && corepack prepare pnpm@11.9.0 --activate \
 && pnpm config set registry https://registry.npmmirror.com \
 && pnpm config set fetch-retries 5 \
 && pnpm config set fetch-retry-mintimeout 20000 \
 && pnpm config set fetch-retry-maxtimeout 120000 \
 && pnpm config set network-concurrency 4
WORKDIR /app

FROM pnpm AS deps
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/portal/package.json ./apps/portal/package.json
COPY apps/web/package.json ./apps/web/package.json
COPY apps/console/package.json ./apps/console/package.json
COPY apps/study-legacy-admin/package.json ./apps/study-legacy-admin/package.json
COPY products/quizcraft/web-app/package.json ./products/quizcraft/web-app/package.json
# Prefer linux/x64/musl optional deps via package.json config for this install only.
RUN node -e 'const fs=require("fs");const p=JSON.parse(fs.readFileSync("package.json","utf8"));p.pnpm=p.pnpm||{};p.pnpm.supportedArchitectures={os:["linux","current"],cpu:["x64","current"],libc:["musl","current"]};fs.writeFileSync("package.json",JSON.stringify(p,null,2));' \
 && pnpm install --frozen-lockfile --filter @henukit/portal...

FROM pnpm AS builder
ARG NEXT_PUBLIC_PORTAL_GATEWAY_URL=
ARG NEXT_PUBLIC_PORTAL_GATEWAY_BASE_URL=/api
ARG NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=1
ARG NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS=0
ENV NEXT_PUBLIC_PORTAL_GATEWAY_URL=$NEXT_PUBLIC_PORTAL_GATEWAY_URL
ENV NEXT_PUBLIC_PORTAL_GATEWAY_BASE_URL=$NEXT_PUBLIC_PORTAL_GATEWAY_BASE_URL
ENV NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=$NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY
ENV NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS=$NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/apps/portal/node_modules ./apps/portal/node_modules
COPY . .
RUN pnpm --filter @henukit/portal run build

FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3001
ENV HOSTNAME=0.0.0.0
COPY --from=builder /app/apps/portal/.next/standalone ./
COPY --from=builder /app/apps/portal/.next/static ./apps/portal/.next/static
COPY --from=builder /app/apps/portal/public ./apps/portal/public
EXPOSE 3001
CMD ["node", "apps/portal/server.js"]
