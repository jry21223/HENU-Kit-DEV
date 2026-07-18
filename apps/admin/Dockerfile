FROM node:22-alpine AS pnpm
RUN corepack enable && corepack prepare pnpm@11.9.0 --activate
WORKDIR /app

FROM pnpm AS deps
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json ./apps/web/package.json
COPY apps/admin/package.json ./apps/admin/package.json
RUN pnpm install --frozen-lockfile --filter @final-review/admin...

FROM pnpm AS builder
ARG VITE_API_BASE_URL
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/apps/admin/node_modules ./apps/admin/node_modules
COPY . .
RUN pnpm --filter @final-review/admin run build

FROM pnpm AS runner
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json ./apps/web/package.json
COPY apps/admin/package.json ./apps/admin/package.json
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/apps/admin/node_modules ./apps/admin/node_modules
COPY --from=builder /app/apps/admin/dist ./apps/admin/dist
EXPOSE 4173
CMD ["pnpm", "--filter", "@final-review/admin", "exec", "vite", "preview", "--host", "0.0.0.0", "--port", "4173"]
