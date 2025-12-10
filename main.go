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
	"strings"
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
	stats := StatsResponse{
		TotalItems:      0,
		TotalCategories: 0,
		TotalPrice:      0,
	}

	defer func() {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
	}()

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		log.Printf("parse multipart error: %v", err)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		log.Printf("form file error: %v", err)
		return
	}
	defer file.Close()

	buf, err := io.ReadAll(file)
	if err != nil {
		log.Printf("read file error: %v", err)
		return
	}

	archiveType := r.URL.Query().Get("type")
	if archiveType != "" && archiveType != "zip" && archiveType != "tar" {
		log.Printf("unknown archive type: %s", archiveType)
		return
	}

	if archiveType == "tar" {
		log.Printf("tar upload received, skipping processing for simple level")
		return
	}

	zipReader, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		log.Printf("zip open error: %v", err)
		return
	}

	var csvFile *zip.File
	for _, f := range zipReader.File {
		if strings.HasSuffix(f.Name, ".csv") {
			csvFile = f
			break
		}
	}
	if csvFile == nil {
		log.Printf("no CSV in ZIP")
		return
	}

	rc, err := csvFile.Open()
	if err != nil {
		log.Printf("csv open error: %v", err)
		return
	}
	defer rc.Close()

	reader := csv.NewReader(rc)

	header, err := reader.Read()
	if err != nil {
		log.Printf("csv header read error: %v", err)
		return
	}
	if len(header) != 5 {
		log.Printf("csv header wrong length: %d", len(header))
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("db begin error: %v", err)
		return
	}

	stmt, err := tx.Prepare(`
		INSERT INTO prices (product_id, name, category, price, create_date)
		VALUES ($1, $2, $3, $4, $5)
	`)
	if err != nil {
		log.Printf("prepare stmt error: %v", err)
		_ = tx.Rollback()
		return
	}
	defer stmt.Close()

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("csv row read error: %v", err)
			_ = tx.Rollback()
			return
		}

		id, err := strconv.Atoi(row[0])
		if err != nil {
			log.Printf("id parse error: %v", err)
			_ = tx.Rollback()
			return
		}

		name := row[1]
		category := row[2]

		price, err := strconv.ParseFloat(row[3], 64)
		if err != nil {
			log.Printf("price parse error: %v", err)
			_ = tx.Rollback()
			return
		}

		date, err := time.Parse("2006-01-02", row[4])
		if err != nil {
			log.Printf("date parse error: %v", err)
			_ = tx.Rollback()
			return
		}

		if _, err = stmt.Exec(id, name, category, price, date); err != nil {
			log.Printf("db insert error: %v", err)
			_ = tx.Rollback()
			return
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("db commit error: %v", err)
		return
	}

	row := db.QueryRow(`
		SELECT
			COUNT(*) AS total_items,
			COUNT(DISTINCT category) AS total_categories,
			COALESCE(SUM(price), 0)
		FROM prices
	`)
	if err := row.Scan(&stats.TotalItems, &stats.TotalCategories, &stats.TotalPrice); err != nil {
		log.Printf("stats query error: %v", err)
		return
	}
}

func handleGet(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT product_id, name, category, price, create_date
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
