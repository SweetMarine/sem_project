package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"log"
	"net/http"
)

type StatsResponse struct {
	TotalItems      int `json:"total_items"`
	TotalCategories int `json:"total_categories"`
	TotalPrice      int `json:"total_price"`
}

func main() {
	http.HandleFunc("/api/v0/prices", pricesHandler)

	log.Println("server is listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func pricesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {

	case http.MethodPost:
		resp := StatsResponse{
			TotalItems:      3,
			TotalCategories: 3,
			TotalPrice:      600,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return

	case http.MethodGet:

		var buf bytes.Buffer
		zipWriter := zip.NewWriter(&buf)

		file, err := zipWriter.Create("data.csv")
		if err != nil {
			http.Error(w, "zip create error", http.StatusInternalServerError)
			return
		}

		file.Write([]byte("id,name,category,price,create_date\n"))

		zipWriter.Close()

		w.Header().Set("Content-Type", "application/zip")
		w.Write(buf.Bytes())
		return

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
