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
COPY products/quizcraft/web-app/package.json ./products/quizcraft/web-app/package.json
COPY apps/portal/package.json ./apps/portal/package.json
COPY apps/web/package.json ./apps/web/package.json
COPY apps/console/package.json ./apps/console/package.json
COPY apps/study-legacy-admin/package.json ./apps/study-legacy-admin/package.json
RUN node -e 'const fs=require("fs");const p=JSON.parse(fs.readFileSync("package.json","utf8"));p.pnpm=p.pnpm||{};p.pnpm.supportedArchitectures={os:["linux","current"],cpu:["x64","current"],libc:["musl","current"]};fs.writeFileSync("package.json",JSON.stringify(p,null,2));' \
 && pnpm install --frozen-lockfile --filter quiz-app...

FROM pnpm AS builder
ARG VITE_BASE_PATH=/quiz/
ARG VITE_API_BASE_URL=/quiz-api/api
ARG VITE_APP_MODE=full
ENV VITE_BASE_PATH=$VITE_BASE_PATH
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
ENV VITE_APP_MODE=$VITE_APP_MODE
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/products/quizcraft/web-app/node_modules ./products/quizcraft/web-app/node_modules
COPY . .
WORKDIR /app/products/quizcraft/web-app
RUN npx tsc && npx vite build

FROM nginx:1.27-alpine AS runner
COPY products/quizcraft/web-app/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/products/quizcraft/web-app/dist /usr/share/nginx/html/quiz
EXPOSE 80
