FROM node:22-alpine AS pnpm
RUN corepack enable && corepack prepare pnpm@11.9.0 --activate
WORKDIR /app

FROM pnpm AS deps
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json ./apps/web/package.json
COPY apps/console/package.json ./apps/console/package.json
COPY apps/study-legacy-admin/package.json ./apps/study-legacy-admin/package.json
RUN pnpm install --frozen-lockfile --filter @henukit/study-legacy-admin...

FROM pnpm AS builder
ARG VITE_API_BASE_URL
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/apps/study-legacy-admin/node_modules ./apps/study-legacy-admin/node_modules
COPY . .
RUN pnpm --filter @henukit/study-legacy-admin run build

FROM pnpm AS runner
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json ./apps/web/package.json
COPY apps/console/package.json ./apps/console/package.json
COPY apps/study-legacy-admin/package.json ./apps/study-legacy-admin/package.json
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/apps/study-legacy-admin/node_modules ./apps/study-legacy-admin/node_modules
COPY --from=builder /app/apps/study-legacy-admin/dist ./apps/study-legacy-admin/dist
EXPOSE 4173
CMD ["pnpm", "--filter", "@henukit/study-legacy-admin", "exec", "vite", "preview", "--host", "0.0.0.0", "--port", "4173"]
