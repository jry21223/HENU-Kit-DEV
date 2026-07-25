FROM node:22-alpine AS pnpm
RUN corepack enable && corepack prepare pnpm@11.9.0 --activate
WORKDIR /app

FROM pnpm AS deps
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY products/quizcraft/web-app/package.json ./products/quizcraft/web-app/package.json
COPY apps/portal/package.json ./apps/portal/package.json
COPY apps/web/package.json ./apps/web/package.json
COPY apps/console/package.json ./apps/console/package.json
COPY apps/study-legacy-admin/package.json ./apps/study-legacy-admin/package.json
RUN pnpm install --frozen-lockfile --filter quiz-app...

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
# Skip prebuild admin-session checks in image builds; they need a live API.
RUN npx tsc && npx vite build

FROM nginx:1.27-alpine AS runner
COPY products/quizcraft/web-app/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/products/quizcraft/web-app/dist /usr/share/nginx/html/quiz
EXPOSE 80
