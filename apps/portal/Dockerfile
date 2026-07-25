FROM node:22-alpine AS pnpm
RUN corepack enable && corepack prepare pnpm@11.9.0 --activate
WORKDIR /app

FROM pnpm AS deps
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/portal/package.json ./apps/portal/package.json
COPY apps/web/package.json ./apps/web/package.json
COPY apps/console/package.json ./apps/console/package.json
COPY apps/study-legacy-admin/package.json ./apps/study-legacy-admin/package.json
COPY products/quizcraft/web-app/package.json ./products/quizcraft/web-app/package.json
RUN pnpm install --frozen-lockfile --filter @henukit/portal...

FROM pnpm AS builder
ARG NEXT_PUBLIC_PORTAL_GATEWAY_BASE_URL=/api
ENV NEXT_PUBLIC_PORTAL_GATEWAY_BASE_URL=$NEXT_PUBLIC_PORTAL_GATEWAY_BASE_URL
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
