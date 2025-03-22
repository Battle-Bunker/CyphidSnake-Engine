
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

var persistentServer *PersistentBoardServer
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
	persistentServer = NewPersistentBoardServer()
	
	router := mux.NewRouter()
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

	gameID, err := commands.PlayBattlesnakeGame(req.Players)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error playing game: %v", err), http.StatusInternalServerError)
		return
	}

	// Create new game in persistent server
	game := board.Game{
		ID:     gameID,
		Status: "running",
		Width:  11,  // Set appropriate values
		Height: 11,
		Source: "API",
	}
	persistentServer.AddGame(game)

	fmt.Printf("Game %v started\n", gameID)

	boardURL := os.Getenv("BOARD_URL")
	if boardURL == "" {
		boardURL = "https://board.battlesnake.com"
	}
	gameURL := fmt.Sprintf("%s/?game=%s", boardURL, gameID)

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(gameURL))
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
	defer ws.Close()

	eventsChan, err := persistentServer.SubscribeToGame(gameID)
	if err != nil {
		ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, err.Error()))
		return
	}

	for event := range eventsChan {
		if err := ws.WriteJSON(event); err != nil {
			if !strings.Contains(err.Error(), "websocket: close") {
				log.Printf("Websocket write error: %v", err)
			}
			break
		}
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	response := IndexResponse{Status: "Ok"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
