-- +goose Up
-- +goose StatementBegin
CREATE TABLE delegates (
    delegate_id INT PRIMARY KEY,           -- Код из st email делегата (например, 100111)
    telegram_id BIGINT UNIQUE,             -- ID делегата в Telegram
    name TEXT,                             -- Имя делегата
    delegate_group TEXT UNIQUE,            -- Уникальная группа делегата
    has_voted BOOLEAN DEFAULT FALSE        -- Флаг: проголосовал ли делегат
);

CREATE TABLE candidates (
    candidate_id INT PRIMARY KEY,          -- Код из st email кандидата
    name TEXT NOT NULL,                    -- Имя кандидата
    course TEXT NOT NULL,                  -- Курс кандидата (например, 1 курс бакалавриата)
    description TEXT,                      -- Описание кандидата
    is_eligible BOOLEAN DEFAULT TRUE       -- Флаг: допущен ли кандидат до выборов
);

CREATE TABLE votes (
    id SERIAL PRIMARY KEY,
    delegate_id INT NOT NULL REFERENCES delegates(delegate_id) ON DELETE CASCADE,   -- Ссылка на делегата через шестизначный код
    candidate_rankings INT[] NOT NULL,                                              -- Массив ранжирования кандидатов
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,                                 -- Время голосования
    UNIQUE (delegate_id)                                                            -- Ограничение на уникальность delegate_id
);

CREATE TABLE results (
    id SERIAL PRIMARY KEY,
    course TEXT NOT NULL UNIQUE,        -- Вакантное место
    winner_candidate_id INT[] NOT NULL, -- Победители
    preferences TEXT,                   -- Парные предпочтения
    strongest_paths TEXT,               -- Сильнейшие пути
    stage TEXT                          -- Как посчитаны результы
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS candidates CASCADE;
DROP TABLE IF EXISTS delegates CASCADE;
DROP TABLE IF EXISTS votes CASCADE;
DROP TABLE IF EXISTS results CASCADE;
-- +goose StatementEnd
