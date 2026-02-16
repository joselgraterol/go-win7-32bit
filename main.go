package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver (No CGO required!)
)

var (
	lastHeartbeat time.Time
	mu            sync.Mutex
	port          = "8888"
	dbName        = "database.db"
)

func initDB() *sql.DB {
	// Put the DB in the same folder as the executable
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	dbPath := filepath.Join(filepath.Dir(ex), dbName)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS logs (id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);`
	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatal(err)
	}
	return db
}

func monitorHeartbeat() {
	// Give the app 10 seconds to open the browser before we start strictly checking
	time.Sleep(10 * time.Second)

	for {
		time.Sleep(2 * time.Second)
		mu.Lock()
		elapsed := time.Since(lastHeartbeat)
		mu.Unlock()

		// If 10 seconds pass without a ping, shut down
		if elapsed > 10*time.Second {
			os.Exit(0)
		}
	}
}

func heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		mu.Lock()
		lastHeartbeat = time.Now()
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

func indexHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		// Log the visit
		_, err := db.Exec("INSERT INTO logs DEFAULT VALUES")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Count launches
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM logs").Scan(&count)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		html := fmt.Sprintf(`
		<!DOCTYPE html>
		<html>
		<head>
			<title>Go Desktop App</title>
			<style>
				body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; text-align: center; padding: 50px; background-color: #f4f4f9; }
				.card { background: white; padding: 30px; border-radius: 10px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); display: inline-block; }
				h1 { color: #333; }
				.counter { font-size: 2em; color: #007bff; font-weight: bold; }
			</style>
			<script>
				let failCount = 0;
				function sendHeartbeat() {
					fetch('/heartbeat', { method: 'POST', cache: 'no-store' })
					.then(response => { if(response.ok) failCount = 0; })
					.catch(err => {
						failCount++;
						if(failCount > 3) console.log("Server not responding");
					});
				}
				setInterval(sendHeartbeat, 2000);
				window.onload = sendHeartbeat;
			</script>
		</head>
		<body>
			<div class="card">
				<h1>Go + SQLite Desktop App</h1>
				<p>Running entirely from a single executable!</p>
				<div class="counter">Total Launches: %d</div>
				<p><small>Close this tab/browser to exit the application.</small></p>
			</div>
		</body>
		</html>`, count)

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}
}

func main() {
	// 1. Initialize DB
	db := initDB()
	defer db.Close()

	// 2. Setup initial heartbeat
	mu.Lock()
	lastHeartbeat = time.Now()
	mu.Unlock()

	// 3. Start heartbeat monitor in the background (Goroutine)
	go monitorHeartbeat()

	// 4. Register Routes
	http.HandleFunc("/heartbeat", heartbeatHandler)
	http.HandleFunc("/", indexHandler(db))

	// 5. Open Browser after a short delay (Windows specific command)
	go func() {
		time.Sleep(1 * time.Second)
		url := "http://localhost:" + port
		exec.Command("cmd", "/c", "start", url).Start()
	}()

	// 6. Start Server
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, nil))
}