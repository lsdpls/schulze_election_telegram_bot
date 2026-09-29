FROM node:18-alpine
WORKDIR /app
COPY package.json ./
# package-lock.json нет: install без audit/fund, чтобы сборка не зависела от сети к advisory-сервису
RUN npm install --no-audit --no-fund
COPY . .
RUN npm run build
EXPOSE 7070
# -s: SPA-режим, любой неизвестный путь → index.html; nginx срезает префикс /votes/
CMD ["npx", "serve", "-s", "build", "-l", "7070"]
