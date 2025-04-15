package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/BattlesnakeOfficial/rules/board"
	"github.com/BattlesnakeOfficial/rules/cli/commands"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var persistentServer *board.PersistentBoardServer
var gameID string
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type PlayRequest struct {
	Players []commands.Player `json:"players"`
}

type IndexResponse struct {
	Status string `json:"status"`
}

func main() {
	persistentServer = board.NewPersistentBoardServer()

	router := mux.NewRouter()

	// Add CORS middleware
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "*")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}

	router.Use(corsMiddleware)
	router.HandleFunc("/play", playHandler).Methods("POST")
	router.HandleFunc("/games/{gameID}", gameHandler).Methods("GET")
	router.HandleFunc("/games/{gameID}/events", eventsHandler).Methods("GET")
	router.HandleFunc("/", indexHandler).Methods("GET")

	fmt.Println("Server is running on http://0.0.0.0:8080")
	log.Fatal(http.ListenAndServe("0.0.0.0:8080", router))
}

func playHandler(w http.ResponseWriter, r *http.Request) {
	var req PlayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tempFile := fmt.Sprintf("/tmp/battlesnake-%s.json", gameID)
	gameID, err := commands.PlayBattlesnakeGame(req.Players, tempFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error playing game: %v", err), http.StatusInternalServerError)
		return
	}

	// Create new game in persistent server
	game := board.Game{
		ID:     gameID,
		Status: "running",
		Width:  11, // Set appropriate values
		Height: 11,
		Source: "API",
	}
	persistentServer.AddGame(game)

	fmt.Printf("Game %v started\n", gameID)

	boardURL := os.Getenv("BOARD_URL")
	if boardURL == "" {
		boardURL = "https://board.battlesnake.com/"
	}
	gameURL := fmt.Sprintf("%s?game=%s", boardURL, gameID)

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(gameURL + "\n"))
}

func gameHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gameID := vars["gameID"]

	game, err := persistentServer.GetGame(gameID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Game *board.Game `json:"game"`
	}{game})
}

func eventsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gameID := vars["gameID"]

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Websocket upgrade failed: %v", err)
		return
	}
	defer func() {
		fmt.Printf("Closing WebSocket for game %v \n", gameID)
		ws.Close()
	}()
	fmt.Printf("Websocket connection for GameID %v is established \n", gameID)

	eventsChan, err := persistentServer.SubscribeToGame(gameID)
	if err != nil {
		fmt.Printf("ERROR! Couldn't subscribe to game\n")
		ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, err.Error()))
		return
	}

	for event := range eventsChan {
		fmt.Printf("Sending event to WebSocket \n")
		if err := ws.WriteJSON(event); err != nil {
			if !strings.Contains(err.Error(), "websocket: close") {
				log.Printf("Websocket write error: %v", err)
			}
			break
		}
	}

	fmt.Printf("All events have been written \n")
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	response := IndexResponse{Status: "Ok"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
