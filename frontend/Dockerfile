FROM node:18-alpine
WORKDIR /app
COPY package.json ./
RUN npm install
COPY . .
RUN npm run build
EXPOSE 7070
CMD ["npx", "serve", "-s", "build", "-l", "7070"]
