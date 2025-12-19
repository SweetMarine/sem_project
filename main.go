package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type StatsResponse struct {
	// Количество успешно добавленных элементов в текущей загрузке
	TotalItems int `json:"total_items"`
	// Общее количество категорий по всей БД
	TotalCategories int `json:"total_categories"`
	// Суммарная стоимость по всей БД
	TotalPrice float64 `json:"total_price"`
}

type priceRow struct {
	Name     string
	Category string
	Price    float64
	Date     time.Time
}

func main() {
	db, err := openDBFromEnv()
	if err != nil {
		log.Fatalf("failed to open DB: %v", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("failed to close DB: %v", cerr)
		}
	}()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping DB: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v0/prices", pricesHandler(db))

	log.Println("server is listening on :8080")

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Printf("server stopped with error: %v", err)
	}
}

func openDBFromEnv() (*sql.DB, error) {
	host := getenv("POSTGRES_HOST", "localhost")
	port := getenv("POSTGRES_PORT", "5432")
	dbname := getenv("POSTGRES_DB", "project-sem-1")
	user := getenv("POSTGRES_USER", "validator")
	pass := getenv("POSTGRES_PASSWORD", "val1dat0r")

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		user, pass, host, port, dbname,
	)
	return sql.Open("postgres", dsn)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func pricesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handlePost(db, w, r)
		case http.MethodGet:
			handleGet(db, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handlePost(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	// multipart/form-data with field "file"
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "failed to parse multipart form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read archive", http.StatusInternalServerError)
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		http.Error(w, "failed to open zip", http.StatusBadRequest)
		return
	}

	// find any .csv inside zip
	var csvEntry *zip.File
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			csvEntry = f
			break
		}
	}
	if csvEntry == nil {
		http.Error(w, "csv not found in zip", http.StatusBadRequest)
		return
	}

	rc, err := csvEntry.Open()
	if err != nil {
		http.Error(w, "failed to open csv", http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	reader := csv.NewReader(rc)

	// header
	if _, err := reader.Read(); err != nil {
		http.Error(w, "failed to read csv header", http.StatusBadRequest)
		return
	}

	rowsData := make([]priceRow, 0, 128)

	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "failed to read csv row", http.StatusBadRequest)
			return
		}
		if len(rec) < 5 {
			http.Error(w, "invalid csv row", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(rec[1])
		category := strings.TrimSpace(rec[2])

		price, err := strconv.ParseFloat(strings.TrimSpace(rec[3]), 64)
		if err != nil {
			http.Error(w, "invalid price", http.StatusBadRequest)
			return
		}

		dt, err := time.Parse("2006-01-02", strings.TrimSpace(rec[4]))
		if err != nil {
			http.Error(w, "invalid create_date", http.StatusBadRequest)
			return
		}

		rowsData = append(rowsData, priceRow{
			Name:     name,
			Category: category,
			Price:    price,
			Date:     dt,
		})
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "db begin failed", http.StatusInternalServerError)
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.Prepare(`INSERT INTO prices (name, category, price, create_date) VALUES ($1,$2,$3,$4)`)
	if err != nil {
		http.Error(w, "prepare failed", http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	inserted := 0
	for _, row := range rowsData {
		if _, err := stmt.Exec(row.Name, row.Category, row.Price, row.Date); err != nil {
			http.Error(w, "db insert failed", http.StatusInternalServerError)
			return
		}
		inserted++
	}

	var totalCategories int
	var totalPrice float64

	if err := tx.QueryRow(`
		SELECT
			COUNT(DISTINCT category) AS total_categories,
			COALESCE(SUM(price), 0)  AS total_price
		FROM prices
	`).Scan(&totalCategories, &totalPrice); err != nil {
		http.Error(w, "stats query failed", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "commit failed", http.StatusInternalServerError)
		return
	}

	resp := StatsResponse{
		TotalItems:      inserted,
		TotalCategories: totalCategories,
		TotalPrice:      totalPrice,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("failed to write json response: %v", err)
	}
}

func handleGet(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	// sql все записи из БД
	rows, err := db.Query(`
		SELECT id, name, category, price::text, create_date::text
		FROM prices
		ORDER BY id
	`)
	if err != nil {
		http.Error(w, "db query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	f, err := zw.Create("data.csv")
	if err != nil {
		http.Error(w, "zip create failed", http.StatusInternalServerError)
		return
	}

	cw := csv.NewWriter(f)
	if err := cw.Write([]string{"id", "name", "category", "price", "create_date"}); err != nil {
		http.Error(w, "csv write failed", http.StatusInternalServerError)
		return
	}

	for rows.Next() {
		var (
			id       int
			name     string
			category string
			priceTxt string
			dateTxt  string
		)
		if err := rows.Scan(&id, &name, &category, &priceTxt, &dateTxt); err != nil {
			http.Error(w, "row scan failed", http.StatusInternalServerError)
			return
		}

		if err := cw.Write([]string{strconv.Itoa(id), name, category, priceTxt, dateTxt}); err != nil {
			http.Error(w, "csv write failed", http.StatusInternalServerError)
			return
		}
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "rows iteration failed", http.StatusInternalServerError)
		return
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		http.Error(w, "csv write failed", http.StatusInternalServerError)
		return
	}

	if err := zw.Close(); err != nil {
		http.Error(w, "zip close failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	if _, err := w.Write(zipBuf.Bytes()); err != nil {
		log.Printf("failed to write zip response: %v", err)
	}
}
