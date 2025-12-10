package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	_ "github.com/lib/pq"
)

var db *sql.DB

// StatsResponse — структура ответа POST /api/v0/prices
type StatsResponse struct {
	TotalItems      int     `json:"total_items"`
	TotalCategories int     `json:"total_categories"`
	TotalPrice      float64 `json:"total_price"`
}

func main() {
	// jdbc
	dsn := "postgres://validator:val1dat0r@localhost:5432/project-sem-1?sslmode=disable"

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to open DB: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping DB: %v", err)
	}

	http.HandleFunc("/api/v0/prices", pricesHandler)

	log.Println("server is listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func pricesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handleUploadPrices(w, r)
	case http.MethodGet:
		handleDownloadPrices(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte("method not allowed"))
	}
}

// handleUploadPrices — POST /api/v0/prices
func handleUploadPrices(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	if len(body) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		http.Error(w, "failed to read zip archive", http.StatusBadRequest)
		return
	}

	// файл data.csv внутри архива
	var csvFile *zip.File
	for _, f := range zipReader.File {
		if f.Name == "data.csv" {
			csvFile = f
			break
		}
	}

	if csvFile == nil {
		http.Error(w, "data.csv not found in archive", http.StatusBadRequest)
		return
	}

	rc, err := csvFile.Open()
	if err != nil {
		http.Error(w, "failed to open data.csv", http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	reader := csv.NewReader(rc)

	// первая строка загаловка
	if _, err := reader.Read(); err != nil {
		http.Error(w, "failed to read CSV header", http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "failed to begin transaction", http.StatusInternalServerError)
		return
	}

	stmt, err := tx.Prepare(`
		INSERT INTO prices (product_id, created_at, name, category, price)
		VALUES ($1, $2, $3, $4, $5)
	`)
	if err != nil {
		_ = tx.Rollback()
		http.Error(w, "failed to prepare insert", http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = tx.Rollback()
			http.Error(w, "failed to read CSV record", http.StatusBadRequest)
			return
		}

		if len(record) < 5 {
			_ = tx.Rollback()
			http.Error(w, "invalid CSV format", http.StatusBadRequest)
			return
		}

		productIDStr := record[0]
		createdStr := record[1]
		name := record[2]
		category := record[3]
		priceStr := record[4]

		productID, err := strconv.Atoi(productIDStr)
		if err != nil {
			_ = tx.Rollback()
			http.Error(w, "invalid product id", http.StatusBadRequest)
			return
		}

		createdAt, err := time.Parse("2006-01-02", createdStr)
		if err != nil {
			_ = tx.Rollback()
			http.Error(w, "invalid date format", http.StatusBadRequest)
			return
		}

		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil {
			_ = tx.Rollback()
			http.Error(w, "invalid price format", http.StatusBadRequest)
			return
		}

		if _, err := stmt.Exec(productID, createdAt, name, category, price); err != nil {
			_ = tx.Rollback()
			http.Error(w, "failed to insert into DB", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "failed to commit transaction", http.StatusInternalServerError)
		return
	}

	// статистика по всей базе
	var stats StatsResponse
	row := db.QueryRow(`
		SELECT 
			COUNT(*) AS total_items,
			COUNT(DISTINCT category) AS total_categories,
			COALESCE(SUM(price), 0) AS total_price
		FROM prices
	`)
	if err := row.Scan(&stats.TotalItems, &stats.TotalCategories, &stats.TotalPrice); err != nil {
		http.Error(w, "failed to compute stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

// handleDownloadPrices — GET /api/v0/prices
func handleDownloadPrices(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT product_id, created_at, name, category, price
		FROM prices
		ORDER BY product_id
	`)
	if err != nil {
		http.Error(w, "failed to query DB", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	fileWriter, err := zipWriter.Create("data.csv")
	if err != nil {
		http.Error(w, "failed to create file in zip", http.StatusInternalServerError)
		return
	}

	csvWriter := csv.NewWriter(fileWriter)

	// заголовок CSV
	header := []string{"id", "date", "name", "category", "price"}
	if err := csvWriter.Write(header); err != nil {
		http.Error(w, "failed to write CSV header", http.StatusInternalServerError)
		return
	}

	for rows.Next() {
		var (
			productID int
			createdAt time.Time
			name      string
			category  string
			price     float64
		)

		if err := rows.Scan(&productID, &createdAt, &name, &category, &price); err != nil {
			http.Error(w, "failed to scan row", http.StatusInternalServerError)
			return
		}

		record := []string{
			strconv.Itoa(productID),
			createdAt.Format("2006-01-02"),
			name,
			category,
			strconv.FormatFloat(price, 'f', 2, 64),
		}

		if err := csvWriter.Write(record); err != nil {
			http.Error(w, "failed to write CSV record", http.StatusInternalServerError)
			return
		}
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "rows error", http.StatusInternalServerError)
		return
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		http.Error(w, "failed to flush CSV", http.StatusInternalServerError)
		return
	}

	if err := zipWriter.Close(); err != nil {
		http.Error(w, "failed to close zip", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="prices.zip"`)
	if _, err := w.Write(buf.Bytes()); err != nil {
		http.Error(w, "failed to write response", http.StatusInternalServerError)
		return
	}
}
