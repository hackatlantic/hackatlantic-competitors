# syntax=docker/dockerfile:1.7
FROM node:20-alpine AS dependencies

WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=secret,id=custom_ca,required=false \
    if [ -f /run/secrets/custom_ca ]; then export NODE_EXTRA_CA_CERTS=/run/secrets/custom_ca; fi; \
    npm ci

FROM node:20-alpine AS build

WORKDIR /app
ARG NEXT_PUBLIC_API_BASE_URL
ARG NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY
ENV NEXT_PUBLIC_API_BASE_URL=$NEXT_PUBLIC_API_BASE_URL
ENV NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY=$NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY
COPY --from=dependencies /app/node_modules ./node_modules
COPY package.json package-lock.json ./
COPY app ./app
COPY components ./components
COPY lib ./lib
COPY instrumentation.ts ./instrumentation.ts
COPY eslint.config.mjs next-env.d.ts next.config.ts proxy.ts tsconfig.json ./
RUN npm run build

FROM cgr.dev/chainguard/node@sha256:f2a8ed64ec02cef2e53c76d1255d0917e749570af251e32e99f54cda1076cc8d AS runtime

WORKDIR /app
ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
COPY --from=build --chown=65532:65532 /app/.next/standalone ./
COPY --from=build --chown=65532:65532 /app/.next/static ./.next/static
USER 65532:65532
EXPOSE 3000
CMD ["server.js"]
