# Шаг 1: Сборка бинарного файла

# Используем образ Golang для сборки приложения
FROM golang:1.22-alpine AS build

# Устанавливаем рабочую директорию внутри контейнера
WORKDIR /build

# Копируем файлы go.mod и go.sum для загрузки зависимостей
COPY go.mod go.sum ./

# Загружаем все зависимости
RUN go mod download

# Копируем все файлы проекта в рабочую директорию контейнера
COPY . ./

# Устанавливаем сертификаты
RUN apk add ca-certificates
# RUN apk --no-cache add ca-certificates
# Сборка бинарного файла приложения
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/app ./cmd/main.go

# Создаем директорию для логов на этапе сборки
RUN mkdir -p /bin/logs
# Устанавлием часоые пояса
RUN apk add tzdata

# Шаг 2: Создание конечного образа

# Используем минимальный образ scratch для минимизации размера конечного образа
FROM scratch AS final

# Устанавливаем рабочую директорию внутри контейнера
WORKDIR /

# Копируем бинарный файл приложения из этапа сборки
COPY --from=build /bin/app /app

# Копируем сертификаты из alpine
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# Копируем директорию для логов
COPY --from=build /bin/logs /logs
# Копируем и устанавливаем часовой пояс
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
ENV TZ=Europe/Moscow

# Открываем необходимые порты
EXPOSE 8080

# Запускаем приложение
ENTRYPOINT ["/app"]
