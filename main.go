package main

import (
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
	if r.Method == http.MethodPost {
		resp := StatsResponse{
			TotalItems:      3,
			TotalCategories: 3,
			TotalPrice:      600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}
