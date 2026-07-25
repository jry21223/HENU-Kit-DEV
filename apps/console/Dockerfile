FROM node:22-alpine AS pnpm
RUN corepack enable && corepack prepare pnpm@11.9.0 --activate
WORKDIR /app

FROM pnpm AS deps
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/console/package.json ./apps/console/package.json
COPY apps/portal/package.json ./apps/portal/package.json
COPY apps/web/package.json ./apps/web/package.json
COPY apps/study-legacy-admin/package.json ./apps/study-legacy-admin/package.json
COPY products/quizcraft/web-app/package.json ./products/quizcraft/web-app/package.json
COPY packages/design-tokens ./packages/design-tokens
RUN pnpm install --frozen-lockfile --filter @henukit/console...

FROM pnpm AS builder
ARG VITE_BASE_PATH=/console/
ARG VITE_QUIZCRAFT_WORKSHOP_URL=/quiz/extract
ENV VITE_BASE_PATH=$VITE_BASE_PATH
ENV VITE_QUIZCRAFT_WORKSHOP_URL=$VITE_QUIZCRAFT_WORKSHOP_URL
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/apps/console/node_modules ./apps/console/node_modules
COPY . .
RUN pnpm --filter @henukit/console run build

FROM nginx:1.27-alpine AS runner
COPY apps/console/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/apps/console/dist /usr/share/nginx/html/console
EXPOSE 80
