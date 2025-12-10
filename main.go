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

type StatsResponse struct {
	TotalItems      int     `json:"total_items"`
	TotalCategories int     `json:"total_categories"`
	TotalPrice      float64 `json:"total_price"`
}

func main() {
	dsn := "postgres://validator:val1dat0r@localhost:5432/project-sem-1?sslmode=disable"

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to open DB: %v", err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
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
		handlePost(w, r)
	case http.MethodGet:
		handleGet(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	// read multipart file
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	buf, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read file bytes", http.StatusInternalServerError)
		return
	}

	zipReader, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		http.Error(w, "failed to open zip archive", http.StatusBadRequest)
		return
	}

	//  data.csv
	var csvFile *zip.File
	for _, f := range zipReader.File {
		if f.Name == "test_data.csv" || f.Name == "data.csv" {
			csvFile = f
			break
		}
	}
	if csvFile == nil {
		http.Error(w, "data.csv not found in zip", http.StatusBadRequest)
		return
	}

	rc, err := csvFile.Open()
	if err != nil {
		http.Error(w, "failed to open csv file", http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	reader := csv.NewReader(rc)

	// читаем заголовок как в тесте
	header, err := reader.Read()
	if err != nil {
		http.Error(w, "invalid csv header", http.StatusBadRequest)
		return
	}

	if len(header) != 5 {
		http.Error(w, "wrong CSV fields count", http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "db begin failed", http.StatusInternalServerError)
		return
	}

	stmt, err := tx.Prepare(`
		INSERT INTO prices (product_id, name, category, price, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`)
	if err != nil {
		tx.Rollback()
		http.Error(w, "prepare failed", http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			tx.Rollback()
			http.Error(w, "failed to read row", http.StatusBadRequest)
			return
		}

		id, _ := strconv.Atoi(row[0])
		name := row[1]
		category := row[2]
		price, _ := strconv.ParseFloat(row[3], 64)
		date, _ := time.Parse("2006-01-02", row[4])

		_, err = stmt.Exec(id, name, category, price, date)
		if err != nil {
			tx.Rollback()
			http.Error(w, "db insert failed", http.StatusInternalServerError)
			return
		}
	}

	err = tx.Commit()
	if err != nil {
		http.Error(w, "commit failed", http.StatusInternalServerError)
		return
	}

	// агрегируем
	var stats StatsResponse
	row := db.QueryRow(`
		SELECT
			COUNT(*) AS total_items,
			COUNT(DISTINCT category) AS total_categories,
			COALESCE(SUM(price), 0)
		FROM prices
	`)
	if err := row.Scan(&stats.TotalItems, &stats.TotalCategories, &stats.TotalPrice); err != nil {
		http.Error(w, "stats query failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func handleGet(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT product_id, name, category, price, created_at
		FROM prices
		ORDER BY product_id
	`)
	if err != nil {
		http.Error(w, "DB query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	f, err := zipWriter.Create("data.csv")
	if err != nil {
		http.Error(w, "zip create failed", http.StatusInternalServerError)
		return
	}

	csvWriter := csv.NewWriter(f)

	csvWriter.Write([]string{"id", "name", "category", "price", "create_date"})

	for rows.Next() {
		var (
			id       int
			name     string
			category string
			price    float64
			date     time.Time
		)
		rows.Scan(&id, &name, &category, &price, &date)

		record := []string{
			strconv.Itoa(id),
			name,
			category,
			strconv.FormatFloat(price, 'f', 2, 64),
			date.Format("2006-01-02"),
		}

		csvWriter.Write(record)
	}

	csvWriter.Flush()
	zipWriter.Close()

	w.Header().Set("Content-Type", "application/zip")
	w.Write(buf.Bytes())
}
