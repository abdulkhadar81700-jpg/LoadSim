package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

var unstableRequests uint64

func fastHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Fast response")
}

func slowHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(200 * time.Millisecond)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Slow response")
}

func unstableHandler(w http.ResponseWriter, r *http.Request) {
	count := atomic.AddUint64(&unstableRequests, 1)

	// Simulate a service that starts failing under heavy request volume.
	if count%5 == 0 {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, "Simulated server failure")
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Stable response")
}

func main() {
	http.HandleFunc("/fast", fastHandler)
	http.HandleFunc("/slow", slowHandler)
	http.HandleFunc("/unstable", unstableHandler)

	fmt.Println("====================================")
	fmt.Println("       LoadSim Test Server")
	fmt.Println("====================================")
	fmt.Println("Running on http://localhost:8080")
	fmt.Println()
	fmt.Println("Endpoints:")
	fmt.Println("  /fast")
	fmt.Println("  /slow")
	fmt.Println("  /unstable")
	fmt.Println("====================================")

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Server error:", err)
	}
}