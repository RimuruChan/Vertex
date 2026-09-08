FROM node:22-bookworm-slim AS build

WORKDIR /app
RUN corepack enable && corepack prepare pnpm@11.9.0 --activate

COPY ui/package.json ui/pnpm-lock.yaml ui/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY ui/index.html ui/tsconfig.json ui/vite.config.ts ./
COPY ui/src ./src
COPY ui/public ./public
RUN pnpm run build

FROM nginx:1.28-alpine

WORKDIR /vertex
ENV NGINX_ENTRYPOINT_LOCAL_RESOLVERS=1
ENV NGINX_ENVSUBST_FILTER=NGINX_LOCAL_RESOLVERS
COPY deploy/nginx.conf /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /vertex/ui
