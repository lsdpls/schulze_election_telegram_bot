package schulze

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

// SaveResultsToCSV сохраняет результаты голосования в CSV файл.
func (s *Schulze) SaveResultsToCSV(ctx context.Context) error {
	// Получаем результаты из базы данных.
	results, err := s.voteChain.GetAllResults(ctx)
	if err != nil {
		return fmt.Errorf("failed to get results: %w", err)
	}
	// Если нет результатов, возвращаем ошибку
	if len(results) == 0 {
		return fmt.Errorf("no results found")
	}

	// Фиксированный путь к файлу в папке logs
	filePath := filepath.Join("logs", "results.csv")
	var csvFile *os.File
	// Проверяем, существует ли файл
	_, err = os.Stat(filePath)
	if os.IsNotExist(err) {
		// Если файла нет, создаем его
		csvFile, err = os.Create(filePath)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
		// Добавляем BOM для UTF-8
		_, err = csvFile.Write([]byte{0xEF, 0xBB, 0xBF})
		if err != nil {
			return fmt.Errorf("failed to write BOM: %w", err)
		}
		defer csvFile.Close()
	} else {
		// Открываем файл для записи
		csvFile, err = os.OpenFile(filePath, os.O_RDWR|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer csvFile.Close()
	}

	// Создаем CSV writer.
	writer := csv.NewWriter(csvFile)
	defer func() {
		writer.Flush()
		if err := writer.Error(); err != nil {
			logrus.Errorf("Ошибка при записи в CSV: %v", err)
		} else {
			logrus.Info("Все данные успешно записаны в CSV.")
		}
	}()
	writer.Write([]string{"Время записи:", time.Now().Format("15:04:05")})
	// Перебираем все результаты и записываем их в CSV
	for _, result := range results {
		// Заголовок для текущего курса
		writer.Write([]string{"Курс:", result.Course})
		writer.Write([]string{"Состояние:", result.Stage})

		// Победители
		winners := []string{"Победители:"}
		for i, winnerID := range result.WinnerCandidateID {
			winnerName, err := s.voteChain.GetCandidateByCandidateID(context.Background(), result.WinnerCandidateID[i])
			if err != nil {
				return fmt.Errorf("failed to get winner name: %w", err)
			}
			winners = append(winners, fmt.Sprintf("st%s %s", idtos(winnerID), winnerName.Name))
		}
		writer.Write(winners)

		// Выводим таблицу парных предпочтений
		writer.Write([]string{"Таблица предпочтений:"})
		err = writeMatrixToCSV(writer, result.Preferences)
		if err != nil {
			return fmt.Errorf("failed to write preferences: %w", err)
		}

		// Выводим таблицу сильнейших путей
		writer.Write([]string{"Таблица сильнейших путей:"})
		err = writeMatrixToCSV(writer, result.StrongestPaths)
		if err != nil {
			return fmt.Errorf("failed to write strongest paths: %w", err)
		}

		// Пустая строка для разделения результатов
		writer.Write([]string{})
	}

	return nil
}

// writeMatrixToCSV выводит мапу мап в виде таблицы
func writeMatrixToCSV(writer *csv.Writer, matrix map[int]map[int]int) error {
	// Извлекаем список ключей (ID кандидатов)
	candidateOrder := make([]int, 0, len(matrix))
	for key := range matrix {
		candidateOrder = append(candidateOrder, key)
	}
	// Сортируем их
	slices.Sort(candidateOrder)
	logrus.Debugf("candidateOrder: %v", candidateOrder)

	// Первая строка: заголовки с ID кандидатов
	header := []string{""} // пустая первая ячейка
	for _, id := range candidateOrder {
		header = append(header, fmt.Sprintf("%06d", id))
	}
	writer.Write(header)

	// Записываем строки для каждой строки в мапе
	for _, rowID := range candidateOrder {
		row := []string{fmt.Sprintf("%06d", rowID)} // первая ячейка — ID строки
		for _, colID := range candidateOrder {
			if val, ok := matrix[rowID][colID]; ok {
				row = append(row, strconv.Itoa(val)) // добавляем значение
			} else {
				row = append(row, "—") // если значения нет, ставим прочерк
			}
		}
		writer.Write(row)
	}
	return nil
}
